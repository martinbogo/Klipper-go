package serialhdl

import "C"
import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/tarm/serial"

	"goklipper/common/logger"
	"goklipper/common/utils/reflects"
	"goklipper/common/utils/sys"
	"goklipper/internal/pkg/chelper"
	"goklipper/internal/pkg/msgproto"
	"goklipper/internal/pkg/util"
)

// Match the original transport reconnect cadence during MCU identify.
// Real nozzle_mcu bring-up on /dev/ttyS5 has proven sensitive to the delay
// between failed identify attempts, and the original code retries after 5s.
const identifySessionTimeout = 5 * time.Second

// serialAckTimeoutSafety is a last-resort safety net only.
// Python uses completion.wait() with waketime=NEVER, meaning the C serialqueue
// owns the actual deadline. We match that by using a very long timeout so the
// C layer fires async_complete before our Go timer can race and delete the
// pending_notification entry early.
const serialAckTimeoutSafety = 30.0
const serialAckPollInterval = 0.050
const maxIdentifyUnknownLogs = 20

var configureUARTPortRTSFunc = configureUARTPortRTS
var clearHUPCLFunc = util.Clear_hupcl
var reconfigureSerialPortBaudFunc = reconfigureSerialPortBaud
var stk500v2LeaveFunc = Stk500v2Leave
var setExclusiveFunc = setTIOCEXCL

type SerialReader struct {
	last_notify_id        int64
	nextHandlerID         uint64
	reactor               Reactor
	warn_prefix           string
	serial_dev            SerialDeviceBase
	Msgparser             *msgproto.MessageParser
	ffi_lib               interface{}
	default_cmd_queue     interface{}
	Serialqueue           interface{}
	stats_buf             []byte
	lock                  sync.Locker
	background_thread     interface{}
	handlers              map[string]interface{}
	pending_notifications sync.Map
	queueLock             sync.Locker
	identifyUnknownCounts map[int]uint32
}

type ownedResponseHandler struct {
	id       uint64
	callback func(map[string]interface{}) error
}

func NewSerialReader(reactor Reactor, warn_prefix string) *SerialReader {
	self := SerialReader{}
	self.reactor = reactor
	self.warn_prefix = warn_prefix
	self.serial_dev = nil
	self.Msgparser = msgproto.NewMessageParser(warn_prefix)
	self.ffi_lib = chelper.Get_ffi()
	self.Serialqueue = nil
	self.default_cmd_queue = self.Alloc_command_queue()
	self.stats_buf = make([]byte, 4096)
	self.lock = &sync.Mutex{}
	self.background_thread = nil
	self.handlers = map[string]interface{}{}
	self.Register_response(self._handle_unknown_init, "#unknown", nil)
	self.Register_response(self.handle_output, "#output", nil)
	self.last_notify_id = 0
	self.queueLock = &sync.Mutex{}
	self.pending_notifications = sync.Map{}
	self.identifyUnknownCounts = map[int]uint32{}
	return &self
}

func (self *SerialReader) _SerialReader() {
	if self.default_cmd_queue != nil {
		chelper.Serialqueue_free_commandqueue(self.default_cmd_queue)
	}
}

func (self *SerialReader) timeoutThread() {
	t := time.Now().Unix()
	for {
		now := time.Now().Unix()
		if now-t > 2 {
			t = now
			self.pending_notifications.Range(func(key, value interface{}) bool {
				pn := value.(Completion)
				if now-pn.CreatedAtUnix() > 60 || !self.reactor.IsRunning() {
					log.Println("serial timeout by 60", pn.CreatedAtUnix())
					self.pending_notifications.Delete(key)
					pn.Complete(nil)
				}
				return true
			})
		} else {
			if self.default_cmd_queue == nil {
				break
			}
			time.Sleep(time.Second)
		}
	}
}
func (self *SerialReader) bg_thread(sq interface{}, done chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for {
		if self.taskThread(sq) {
			break
		}
	}
	if done != nil {
		close(done)
	}
}
func (self *SerialReader) taskThread(sq interface{}) bool {
	defer sys.CatchPanic()
	response := chelper.New_pull_queue_message()
	defer func() {
		runtime.SetFinalizer(response, func(t interface{}) {
			if self.default_cmd_queue != nil {
				chelper.Serialqueue_free_commandqueue(self.default_cmd_queue)
				self.default_cmd_queue = nil
			}
		})
	}()
	for {
		chelper.Serialqueue_pull(sq, response)
		count := response.Len
		if count < 0 {
			return true
		}
		if response.Notify_id > 0 {
			params := map[string]interface{}{"#sent_time": response.Sent_time,
				"#receive_time": response.Receive_time}
			completion, ok := self.pending_notifications.Load(int64(response.Notify_id))
			if ok {
				self.pending_notifications.Delete(int64(response.Notify_id))
				self.reactor.AsyncComplete(completion.(Completion), params)
			}
			continue
		}
		msgs := []int{}
		for i := 0; i < int(count); i++ {
			msgs = append(msgs, int(response.Msg[i]))
		}
		params := self.Msgparser.Parse(msgs)
		params["#sent_time"] = response.Sent_time
		params["#receive_time"] = response.Receive_time
		self.dispatchParsedParams(params)
	}
}
func (self *SerialReader) dispatchParsedParams(params map[string]interface{}) {
	hdl, ok := params["#name"]
	if !ok {
		return
	}
	if hdl1, ok1 := params["oid"]; ok1 {
		hdl = strings.Join([]string{hdl.(string), strconv.FormatInt(hdl1.(int64), 10)}, "")
	}
	self.lock.Lock()
	defer self.lock.Unlock()
	handler := self.handlers[hdl.(string)]
	if handler == nil {
		return
	}
	var err error
	switch fn := handler.(type) {
	case func(map[string]interface{}) error:
		err = fn(params)
	case ownedResponseHandler:
		err = fn.callback(params)
	default:
		logger.Errorf("%sUnsupported serial callback type %T", self.warn_prefix, handler)
		return
	}
	if err != nil {
		logger.Errorf("%sException in serial callback", self.warn_prefix)
	}
}
func (self *SerialReader) _error(msg string, params map[string]interface{}) {
	panic(fmt.Sprintf("%s%s%s", self.warn_prefix, msg, params))
}

func (self *SerialReader) _get_identify_data_sync() interface{} {
	identify_data := []byte{}
	for {
		msg := fmt.Sprintf("identify offset=%d count=%d", len(identify_data), 40)
		params, err := self.Send_with_response(msg, "identify_response")
		if err != nil {
			return nil
		}
		if params["offset"].(int64) == int64(len(identify_data)) {
			msgdata, ok := params["data"]
			if !ok {
				return identify_data
			}
			bs, ok := msgdata.([]int)
			if ok && len(bs) == 0 {
				return identify_data
			}
			for _, i := range msgdata.([]int) {
				identify_data = append(identify_data, byte(i))
			}
		}
	}
}

// _get_identify_data runs on the reactor callback path when the reactor is
// active, mirroring Python's register_callback() + completion.wait() flow.
func (self *SerialReader) _get_identify_data(eventtime interface{}) interface{} {
	_ = eventtime
	return self._get_identify_data_sync()
}

// _get_identify_data_goroutine is used when the reactor is not active and we
// must drive identify from a plain goroutine.
func (self *SerialReader) _get_identify_data_goroutine(result chan<- interface{}) {
	result <- self._get_identify_data_sync()
}

func (self *SerialReader) _start_session(serial_dev SerialDeviceBase, serial_fd_type byte, client_id int) bool {
	self.identifyUnknownCounts = map[int]uint32{}
	self.serial_dev = serial_dev
	self.queueLock.Lock()
	self.Serialqueue = chelper.Serialqueue_alloc(serial_dev.GetFd(), serial_fd_type, client_id)
	sq := self.Serialqueue
	bg_thread_done := make(chan struct{})
	self.background_thread = bg_thread_done
	self.queueLock.Unlock()
	go self.bg_thread(sq, bg_thread_done)
	go self.timeoutThread()
	var identify_data interface{}
	if self.reactor.IsRunning() {
		completion := self.reactor.RegisterCallback(self._get_identify_data, 0)
		identify_data = completion.Wait(self.reactor.Monotonic()+identifySessionTimeout.Seconds(), nil)
	} else {
		resultCh := make(chan interface{}, 1)
		go self._get_identify_data_goroutine(resultCh)
		select {
		case identify_data = <-resultCh:
		case <-time.After(identifySessionTimeout):
		}
	}
	if identify_data == nil {
		logger.Errorf("%sTimeout on connect", self.warn_prefix)
		self.Disconnect()
		time.Sleep(time.Second)
		return false
	}
	msgparser := msgproto.NewMessageParser(self.warn_prefix)
	msgparser.Process_identify(identify_data.([]byte), true)
	self.Msgparser = msgparser
	self.Register_response(self.Handle_unknown, "#unknown", nil)
	wire_freq := 2500000.
	if serial_fd_type == 'c' {
		wire_freq = msgparser.Get_constant_float("CANBUS_FREQUENCY", nil)
	} else {
		wire_freq = msgparser.Get_constant_float("SERIAL_BAUD", nil)
	}
	if wire_freq != 0 {
		chelper.Serialqueue_set_wire_frequency(self.Serialqueue, wire_freq)
	}
	receive_window := msgparser.Get_constant_int("RECEIVE_WINDOW", nil)
	if receive_window != 0 {
		chelper.Serialqueue_set_receive_window(self.Serialqueue, receive_window)
	}
	return true
}

func (self *SerialReader) connect_canbus(canbus_uuid, canbus_nodeid, canbus_iface string) {
}

func (self *SerialReader) ConnectCanbus(canbus_uuid, canbus_nodeid, canbus_iface string) {
	self.connect_canbus(canbus_uuid, canbus_nodeid, canbus_iface)
}

func (self *SerialReader) Connect_pipe(filename string) {
	logger.Infof("%s Starting connect", self.warn_prefix)
	start_time := self.reactor.Monotonic()
	for {
		if self.reactor.Monotonic() > start_time+90. {
			self._error("Unable to connect", nil)
		}
		fd, err := syscall.Open(filename, syscall.O_RDWR|syscall.O_NOCTTY, 0777)
		if err != nil {
			logger.Errorf("%sUnable to open port: %s", self.warn_prefix, err)
			self.reactor.Pause(self.reactor.Monotonic() + 5.)
			continue
		}
		serial_dev := os.NewFile(uintptr(fd), filename)
		serialDev := NewSerialDev(nil, nil, serial_dev)
		ret := self._start_session(serialDev, 'u', 0)
		if ret {
			break
		}
	}
}
func (self *SerialReader) Connect_uart(serialport string, baud int, rts bool) {
	logger.Infof("%sStarting serial connect: %s, %d", self.warn_prefix, serialport, baud)
	start_time := self.reactor.Monotonic()
	for {
		if self.reactor.Monotonic() > start_time+90. {
			self._error("Unable to connect", nil)
		}
		cfg := &serial.Config{Name: serialport, Baud: baud, ReadTimeout: time.Microsecond * 900}
		serial_dev, err := serial.OpenPort(cfg)
		if err != nil {
			logger.Errorf("%sUnable to open serial port: %s", self.warn_prefix, err)
			time.Sleep(time.Second)
			self.reactor.Pause(self.reactor.Monotonic() + 5.)
			continue
		}
		serialFile := reflects.GetPrivateFieldValue(serial_dev, "f").(*os.File)
		// Match Python's exclusive=True: set TIOCEXCL so no other process
		// (gkapi, etc.) can open the port while we hold it.
		if exclErr := setExclusiveFunc(serialFile.Fd()); exclErr != nil {
			logger.Warnf("%sUnable to set exclusive lock on serial port: %v", self.warn_prefix, exclErr)
		}
		serialDev := NewSerialDev(serial_dev, cfg, serialFile)
		self.prepareUARTSerialPort(serialDev, rts)
		// Give the MCU's firmware time to drain garbage bytes introduced by
		// the stk500v2 baud-rate switching sequence before we start identify.
		time.Sleep(250 * time.Millisecond)
		ret := self._start_session(serialDev, 'u', 0)
		if ret {
			break
		}
	}
}

func (self *SerialReader) prepareUARTSerialPort(serialDev *SerialDev, rts bool) {
	if err := configureUARTPortRTSFunc(serialDev.file, rts); err != nil {
		logger.Warnf("%sUnable to set RTS=%t on serial port: %v", self.warn_prefix, rts, err)
	}
	stk500v2LeaveFunc(serialDev)
	if serialDev.Port != nil {
		if err := serialDev.Port.Flush(); err != nil {
			logger.Warnf("%sUnable to flush serial port buffers: %v", self.warn_prefix, err)
		}
	}
}

func (self *SerialReader) Connect_remote(serialport string) {
	logger.Debugf("%sStarting remote connect %s", self.warn_prefix, serialport)
	start_time := self.reactor.Monotonic()
	for {
		if self.reactor.Monotonic() > start_time+90. {
			self._error("Unable to connect", nil)
		}
		serialDev, err := NewRemoteDev(serialport)
		if err != nil {
			continue
		}
		if self._start_session(serialDev, 'n', 0) {
			break
		}
	}
}
func (self *SerialReader) Set_clock_est(freq float64, conv_time float64, conv_clock int64, last_clock int64) {
	self.queueLock.Lock()
	sq := self.Serialqueue
	self.queueLock.Unlock()
	if sq == nil {
		return
	}
	chelper.Serialqueue_set_clock_est(sq, freq, conv_time, uint64(conv_clock), uint64(last_clock))
}
func (self *SerialReader) Disconnect() {
	self.queueLock.Lock()
	sq := self.Serialqueue
	self.Serialqueue = nil
	var done chan struct{}
	if self.background_thread != nil {
		done = self.background_thread.(chan struct{})
		self.background_thread = nil
	}
	self.queueLock.Unlock()
	if sq != nil {
		chelper.Serialqueue_exit(sq)
		if done != nil {
			<-done
		}
		chelper.Serialqueue_free(sq)
	}
	if self.serial_dev != nil {
		self.serial_dev.Close()
		self.serial_dev = nil
	}
	self.pending_notifications.Range(func(key, value interface{}) bool {
		pn := value.(Completion)
		pn.Complete(nil)
		return true
	})
	self.pending_notifications = sync.Map{}
}
func (self *SerialReader) stats(eventtime float64) string {
	self.queueLock.Lock()
	sq := self.Serialqueue
	self.queueLock.Unlock()
	if sq == nil {
		return ""
	}
	chelper.Serialqueue_get_stats(sq, self.stats_buf)
	return strings.TrimRight(string(self.stats_buf), "\x00")
}
func (self *SerialReader) Stats(eventtime float64) string {
	return self.stats(eventtime)
}
func (self *SerialReader) Get_reactor() Reactor {
	return self.reactor
}
func (self *SerialReader) Get_msgparser() *msgproto.MessageParser {
	return self.Msgparser
}
func (self *SerialReader) Get_default_command_queue() interface{} {
	return self.default_cmd_queue
}
func (self *SerialReader) Register_response(callback interface{}, name string, oid interface{}) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.registerResponseLocked(callback, name, oid)
}

func (self *SerialReader) registerResponseLocked(callback interface{}, name string, oid interface{}) {
	key := ""
	if oid != nil && oid.(int) >= 0 {
		key = fmt.Sprintf("%s%d", name, oid.(int))
	} else {
		key = name
	}
	if callback == nil {
		delete(self.handlers, key)
	} else {
		self.handlers[key] = callback
	}
}

func (self *SerialReader) getResponseKey(name string, oid interface{}) string {
	if oid != nil && oid.(int) >= 0 {
		return fmt.Sprintf("%s%d", name, oid.(int))
	}
	return name
}

func (self *SerialReader) Register_owned_response(callback func(map[string]interface{}) error, name string, oid interface{}) uint64 {
	handlerID := atomic.AddUint64(&self.nextHandlerID, 1)
	self.lock.Lock()
	defer self.lock.Unlock()
	self.registerResponseLocked(ownedResponseHandler{id: handlerID, callback: callback}, name, oid)
	return handlerID
}

func (self *SerialReader) Unregister_owned_response(name string, oid interface{}, handlerID uint64) {
	key := self.getResponseKey(name, oid)
	self.lock.Lock()
	defer self.lock.Unlock()
	current, ok := self.handlers[key]
	if !ok {
		return
	}
	switch h := current.(type) {
	case ownedResponseHandler:
		if h.id == handlerID {
			delete(self.handlers, key)
		}
	}
}
func (self *SerialReader) Raw_send(cmd []int, minclock, reqclock int64, cmd_queue interface{}) {
	self.queueLock.Lock()
	sq := self.Serialqueue
	self.queueLock.Unlock()
	if sq == nil {
		return
	}
	chelper.Serialqueue_send(sq, cmd_queue, cmd, len(cmd), minclock, reqclock, 0)
}

func (self *SerialReader) isSerialQueueActive() bool {
	self.queueLock.Lock()
	defer self.queueLock.Unlock()
	return self.Serialqueue != nil
}

func (self *SerialReader) Raw_send_wait_ack(cmd []int, minclock, reqclock int64, cmd_queue interface{}) map[string]interface{} {
	return self.rawSendWaitAckWithPoll(cmd, minclock, reqclock, cmd_queue, nil)
}

func (self *SerialReader) rawSendWaitAckWithPoll(cmd []int, minclock, reqclock int64, cmd_queue interface{}, shouldStop func() bool) map[string]interface{} {
	nid := atomic.AddInt64(&self.last_notify_id, 1)
	completion := self.reactor.NewCompletion()
	self.pending_notifications.Store(nid, completion)
	self.queueLock.Lock()
	sq := self.Serialqueue
	self.queueLock.Unlock()
	if sq == nil {
		self.pending_notifications.Delete(nid)
		return nil
	}
	chelper.Serialqueue_send(sq, cmd_queue, cmd, len(cmd), minclock, reqclock, nid)
	deadline := self.reactor.Monotonic() + serialAckTimeoutSafety
	for {
		if shouldStop != nil && shouldStop() {
			self.pending_notifications.Delete(nid)
			return map[string]interface{}{}
		}
		waketime := deadline
		now := self.reactor.Monotonic()
		if shouldStop != nil && now+serialAckPollInterval < waketime {
			waketime = now + serialAckPollInterval
		}
		params := completion.Wait(waketime, nil)
		if params != nil {
			self.pending_notifications.Delete(nid)
			return params.(map[string]interface{})
		}
		if shouldStop != nil && shouldStop() {
			self.pending_notifications.Delete(nid)
			return map[string]interface{}{}
		}
		if self.reactor.Monotonic() >= deadline {
			self.pending_notifications.Delete(nid)
			return nil
		}
	}
}
func (self *SerialReader) Send(msg string, minclock, reqclock int64) {
	cmd := self.Msgparser.Create_command(msg)
	self.Raw_send(cmd, minclock, reqclock, self.default_cmd_queue)
}
func (self *SerialReader) Send_with_response(msg string, response string) (map[string]interface{}, error) {
	cmd := self.Msgparser.Create_command(msg)
	src := NewSerialRetryCommand(self, response, nil)
	return src.get_response([]interface{}{cmd}, self.default_cmd_queue, 0, 0)
}
func (self *SerialReader) Alloc_command_queue() interface{} {
	return chelper.Serialqueue_alloc_commandqueue()
}
func (self *SerialReader) Dump_debug() string {
	out := []string{}
	out = append(out, fmt.Sprintf("Dumping serial stats: %s", self.stats(self.reactor.Monotonic())))
	var scount int
	out = append(out, fmt.Sprintf("Dumping send queue %d messages", scount))
	return strings.Join(out, "\n")
}
func normalizeMsgID(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return -1
	}
}

func (self *SerialReader) trackIdentifyUnknown(msgid int) (uint32, bool) {
	if self.identifyUnknownCounts == nil {
		self.identifyUnknownCounts = map[int]uint32{}
	}
	count := self.identifyUnknownCounts[msgid] + 1
	self.identifyUnknownCounts[msgid] = count
	return count, count <= maxIdentifyUnknownLogs
}

func (self *SerialReader) _handle_unknown_init(params map[string]interface{}) error {
	msgid := normalizeMsgID(params["#msgid"])
	count, shouldLog := self.trackIdentifyUnknown(msgid)
	if !shouldLog {
		return nil
	}
	logger.Debugf("%sUnknown message %d (len %v) while identifying, cnt: %d",
		self.warn_prefix, msgid, params["#msg"], count)
	return nil
}
func (self *SerialReader) Handle_unknown(params map[string]interface{}) error {
	logger.Warnf("%sUnknown message type %d: %s",
		self.warn_prefix, params["#msgid"], params["#msg"])
	return nil
}
func (self *SerialReader) handle_output(params map[string]interface{}) error {
	logger.Warnf("%s%s: %s", self.warn_prefix, params["#name"], params["#msg"])
	return nil
}
func (self *SerialReader) handle_default(params map[string]interface{}) error {
	logger.Warnf("%sgot %s", self.warn_prefix, params)
	return nil
}

type SerialDeviceBase interface {
	Close()
	GetFd() uintptr
}

type serialPort interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	Flush() error
}

type SerialDev struct {
	Port   serialPort
	Config *serial.Config
	file   *os.File
}

func (self *SerialDev) Close() {
	if self.Port != nil {
		self.Port.Close()
	} else if self.file != nil {
		self.file.Close()
	}
}
func (self *SerialDev) GetFd() uintptr {
	return self.file.Fd()
}
func NewSerialDev(port serialPort, config *serial.Config, file *os.File) *SerialDev {
	self := SerialDev{Port: port, Config: config, file: file}
	return &self
}

type RemoteDev struct {
	url  string
	conn *net.TCPConn
	file *os.File
}

func (self *RemoteDev) Close() {
	if self.conn != nil {
		self.conn.Close()
	}
	if self.file != nil {
		self.file.Close()
	}
}
func (self *RemoteDev) GetFd() uintptr {
	return self.file.Fd()
}
func NewRemoteDev(url string) (*RemoteDev, error) {
	addr := strings.Split(url, "tcp@")[1]
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		log.Print("ResolveTCPAddr failed:", err.Error())
		return nil, err
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		println("Dial failed:", err.Error())
		return nil, err
	}
	conn.SetNoDelay(true)
	file, _ := conn.File()
	return &RemoteDev{url, conn, file}, nil
}

type SerialRetryCommand struct {
	serial      *SerialReader
	name        string
	oid         interface{}
	last_params map[string]interface{}
	handlerID   uint64
	sendWaitAck func(cmd []int, minclock, reqclock int64, cmd_queue interface{}, shouldStop func() bool) map[string]interface{}
	sendRaw     func(cmd []int, minclock, reqclock int64, cmd_queue interface{})
}

func NewSerialRetryCommand(serial *SerialReader, name string, oid interface{}) *SerialRetryCommand {
	self := SerialRetryCommand{}
	self.serial = serial
	self.name = name
	self.oid = oid
	self.last_params = nil
	self.sendWaitAck = serial.rawSendWaitAckWithPoll
	self.sendRaw = serial.Raw_send
	self.handlerID = self.serial.Register_owned_response(self.handle_callback, name, oid)
	return &self
}
func (self *SerialRetryCommand) handle_callback(params map[string]interface{}) error {
	self.last_params = params
	return nil
}
func (self *SerialRetryCommand) get_response(cmds []interface{}, cmd_queue interface{}, minclock, reqclock int64) (map[string]interface{}, error) {
	retries := 5
	retry_delay := .010
	for {
		if self.last_params != nil {
			self.serial.Unregister_owned_response(self.name, self.oid, self.handlerID)
			return self.last_params, nil
		}
		lastCmdIndex := len(cmds) - 1
		if lastCmdIndex < 0 {
			return map[string]interface{}{}, nil
		}
		for i, cmd := range cmds {
			if i == lastCmdIndex {
				continue
			}
			self.sendRaw(cmd.([]int), minclock, reqclock, cmd_queue)
		}
		ackParams := self.sendWaitAck(cmds[lastCmdIndex].([]int), minclock, reqclock, cmd_queue, func() bool {
			return self.last_params != nil
		})
		params := self.last_params
		if params != nil {
			self.serial.Unregister_owned_response(self.name, self.oid, self.handlerID)
			return params, nil
		}
		if ackParams == nil && !self.serial.isSerialQueueActive() {
			self.serial.Unregister_owned_response(self.name, self.oid, self.handlerID)
			return nil, errors.New("Serial connection closed")
		}
		if retries <= 0 {
			self.serial.Unregister_owned_response(self.name, self.oid, self.handlerID)
			return nil, errors.New(fmt.Sprintf("Unable to obtain '%s' response", self.name))
		}
		// Use time.Sleep rather than reactor.Pause: Send_with_response is
		// called from the identify goroutine (a plain goroutine, not a reactor
		// greenlet) where reactor.Pause is a no-op. time.Sleep is always
		// correct; reactor.Pause would also work inside a greenlet but is not
		// needed here since these retry delays are very short (10–320ms).
		time.Sleep(time.Duration(retry_delay * float64(time.Second)))
		if self.last_params != nil {
			self.serial.Unregister_owned_response(self.name, self.oid, self.handlerID)
			return self.last_params, nil
		}
		retries -= 1
		retry_delay *= 2.
	}
}

type RetryAsyncCommand struct {
	Completion     Completion
	Reactor        Reactor
	Serial         *SerialReader
	Oid            int
	HandlerID      uint64
	Min_query_time float64
	Name           string
	need_response  bool
	last_params    map[string]interface{}
	sendWaitAck    func(cmd []int, minclock, reqclock int64, cmd_queue interface{}) map[string]interface{}
	sendRaw        func(cmd []int, minclock, reqclock int64, cmd_queue interface{})
}

var TIMEOUT_TIME = 5.0
var RETRY_TIME = 0.500

func (self *RetryAsyncCommand) __init__(serial interface{}, name string, oid int) {
	self.Serial = serial.(*SerialReader)
	self.Name = name
	self.Oid = oid
	self.Reactor = self.Serial.Get_reactor()
	self.Completion = self.Reactor.NewCompletion()
	self.Min_query_time = self.Reactor.Monotonic()
	self.need_response = true
	self.last_params = nil
	self.sendWaitAck = self.Serial.Raw_send_wait_ack
	self.sendRaw = self.Serial.Raw_send
	self.HandlerID = self.Serial.Register_owned_response(self.Handle_callback, name, oid)
}
func (self *RetryAsyncCommand) Handle_callback(params map[string]interface{}) error {
	send_time := chelper.CdoubleTofloat64(params["#sent_time"])
	if self.need_response && send_time >= self.Min_query_time {
		self.need_response = false
		self.last_params = params
		self.Reactor.AsyncComplete(self.Completion, params)
	}
	return nil
}
func (self *RetryAsyncCommand) unregisterAndReturnLastParams() (map[string]interface{}, bool) {
	if self.last_params == nil {
		return nil, false
	}
	self.Serial.Unregister_owned_response(self.Name, self.Oid, self.HandlerID)
	return self.last_params, true
}
func (self *RetryAsyncCommand) get_response(cmds []interface{}, cmd_queue interface{}, minclock, reqclock int64) (map[string]interface{}, error) {
	cmd := cmds[0]
	if self.sendWaitAck(cmd.([]int), minclock, reqclock, cmd_queue) == nil {
		if params, ok := self.unregisterAndReturnLastParams(); ok {
			return params, nil
		}
		self.Serial.Unregister_owned_response(self.Name, self.Oid, self.HandlerID)
		panic(fmt.Sprintf("Timeout on send for '%s' response '%d'", self.Name, self.Oid))
	}
	self.Min_query_time = 0
	first_query_time := self.Reactor.Monotonic()
	query_time := first_query_time
	for {
		if params, ok := self.unregisterAndReturnLastParams(); ok {
			return params, nil
		}
		params := self.Completion.Wait(query_time+RETRY_TIME, nil)
		if params != nil {
			self.Serial.Unregister_owned_response(self.Name, self.Oid, self.HandlerID)
			return params.(map[string]interface{}), nil
		}
		if params, ok := self.unregisterAndReturnLastParams(); ok {
			return params, nil
		}
		query_time = self.Reactor.Monotonic()
		if query_time > first_query_time+TIMEOUT_TIME {
			if params, ok := self.unregisterAndReturnLastParams(); ok {
				return params, nil
			}
			self.Serial.Unregister_owned_response(self.Name, self.Oid, self.HandlerID)
			panic(fmt.Sprintf("Timeout on wait for '%s' response '%d'", self.Name, self.Oid))
		}
		if params, ok := self.unregisterAndReturnLastParams(); ok {
			return params, nil
		}
		self.sendRaw(cmd.([]int), minclock, minclock, cmd_queue)
	}
}

type RetryCommand interface {
	get_response(cmds []interface{}, cmd_queue interface{}, minclock, reqclock int64) (map[string]interface{}, error)
}

type retryCommandFunc func(cmds []interface{}, cmd_queue interface{}, minclock, reqclock int64) (map[string]interface{}, error)

func (fn retryCommandFunc) get_response(cmds []interface{}, cmd_queue interface{}, minclock, reqclock int64) (map[string]interface{}, error) {
	return fn(cmds, cmd_queue, minclock, reqclock)
}

var newSerialRetryCommandFunc = func(serial *SerialReader, name string, oid interface{}) RetryCommand {
	return NewSerialRetryCommand(serial, name, oid)
}

var newRetryAsyncCommandFunc = func(serial *SerialReader, name string, oid int) RetryCommand {
	return NewRetryAsyncCommand(serial, name, oid)
}

func NewRetryAsyncCommand(serial *SerialReader, name string, oid int) *RetryAsyncCommand {
	ob := RetryAsyncCommand{}
	ob.__init__(serial, name, oid)
	return &ob
}

type CommandQueryWrapper struct {
	Xmit_helper bool
	Cmd_queue   interface{}
	Error       func(string) interface{}
	Response    string
	Serial      *SerialReader
	Oid         int
	Cmd         interface{}
}

func NewCommandQueryWrapper(serial *SerialReader, msgformat string, respformat string, oid int, cmd_queue interface{}, is_async bool, error interface{}) *CommandQueryWrapper {
	self := new(CommandQueryWrapper)
	self.Serial = serial
	cmd, err := serial.Get_msgparser().Lookup_command(msgformat)
	if err != nil {
	}
	self.Cmd = cmd
	serial.Get_msgparser().Lookup_command(respformat)
	self.Response = strings.Split(respformat, " ")[0]
	self.Oid = oid
	if errFunc, ok := error.(func(string) interface{}); ok {
		self.Error = errFunc
	}
	self.Xmit_helper = is_async
	if cmd_queue == nil {
		cmd_queue = serial.Get_default_command_queue()
	}
	self.Cmd_queue = cmd_queue
	return self
}
func (self *CommandQueryWrapper) Do_send(cmds [][]int, minclock, reqclock int64) interface{} {
	var xh RetryCommand
	if self.Xmit_helper {
		xh = newRetryAsyncCommandFunc(self.Serial, self.Response, self.Oid)
	} else {
		xh = newSerialRetryCommandFunc(self.Serial, self.Response, self.Oid)
	}
	if reqclock < minclock {
		reqclock = minclock
	}
	cmd := []interface{}{}
	for _, c := range cmds {
		cmd = append(cmd, c)
	}
	res, err := xh.get_response(cmd, self.Cmd_queue, minclock, reqclock)
	if err != nil {
		if self.Error != nil {
			panic(self.Error(err.Error()))
		}
		panic(err)
	}
	return res
}
func (self *CommandQueryWrapper) Send(data interface{}, minclock, reqclock int64) interface{} {
	return self.Do_send([][]int{self.Cmd.(*msgproto.MessageFormat).Encode(data)}, minclock, reqclock)
}
func (self *CommandQueryWrapper) Send_with_preface(preface_cmd *CommandWrapper, preface_data interface{}, data interface{}, minclock, reqclock int64) interface{} {
	cmds := [][]int{preface_cmd.Cmd.Encode(preface_data.([]interface{})), self.Cmd.(*msgproto.MessageFormat).Encode([]interface{}{data})}
	return self.Do_send(cmds, minclock, reqclock)
}

type CommandWrapper struct {
	Cmd_queue interface{}
	Serial    *SerialReader
	Cmd       *msgproto.MessageFormat
}

func NewCommandWrapper(serial *SerialReader, msgformat interface{}, cmd_queue interface{}) (*CommandWrapper, error) {
	self := CommandWrapper{}
	self.Serial = serial
	cmd, err := serial.Get_msgparser().Lookup_command(msgformat.(string))
	if err != nil {
		return nil, err
	}
	if cmd != nil {
		self.Cmd = cmd.(*msgproto.MessageFormat)
	}
	if cmd_queue == nil {
		cmd_queue = serial.Get_default_command_queue()
	}
	self.Cmd_queue = cmd_queue
	return &self, nil
}
func (self *CommandWrapper) Send(data interface{}, minclock, reqclock int64) {
	cmd := self.Cmd.Encode(data)
	self.Serial.Raw_send(cmd, minclock, reqclock, self.Cmd_queue)
}

// Stk500v2Leave attempts to place an AVR stk500v2-style programmer into normal
// mode. Called from Connect_uart's goroutine, NOT from a reactor greenlet, so
// all timing MUST use time.Sleep (reactor.Pause is a no-op outside greenlets).
func Stk500v2Leave(ser *SerialDev) {
	logger.Debug("Starting stk500v2 leave programmer sequence")
	if ser == nil || ser.file == nil || ser.Config == nil || ser.Port == nil {
		logger.Warn("Skipping stk500v2 leave programmer sequence: serial device is incomplete")
		return
	}
	clearHUPCLFunc(ser.file.Fd())
	origbaud := ser.Config.Baud
	portName := ser.Config.Name
	if portName == "" {
		portName = ser.file.Name()
	}
	if err := reconfigureSerialPortBaudFunc(ser.file, portName, 2400); err != nil {
		logger.Warnf("Unable to switch serial port to 2400 baud for stk500v2 leave: %v", err)
		return
	}
	ser.Config.Baud = 2400
	bs := make([]byte, 4096)
	_, err := ser.Port.Read(bs)
	if err != nil {
		logger.Error(err.Error())
	}
	if err := reconfigureSerialPortBaudFunc(ser.file, portName, 115200); err != nil {
		logger.Warnf("Unable to switch serial port to 115200 baud for stk500v2 leave: %v", err)
		if restoreErr := reconfigureSerialPortBaudFunc(ser.file, portName, origbaud); restoreErr == nil {
			ser.Config.Baud = origbaud
		}
		return
	}
	ser.Config.Baud = 115200
	// Match Python's reactor.pause(0.100) — must use time.Sleep here since
	// this function runs in a plain goroutine, not a reactor greenlet.
	time.Sleep(100 * time.Millisecond)
	ser.Port.Read(bs)
	writeData := []byte("\x1b\x01\x00\x01\x0e\x11\x04")
	ser.Port.Write(writeData)
	time.Sleep(50 * time.Millisecond)
	count, err1 := ser.Port.Read(bs)
	if err1 != nil {
		logger.Error(err1.Error())
	}
	logger.Debugf("Got %X from stk500v2", string(bs[:count]))
	if err := reconfigureSerialPortBaudFunc(ser.file, portName, origbaud); err != nil {
		logger.Warnf("Unable to restore serial port baud to %d after stk500v2 leave: %v", origbaud, err)
		return
	}
	ser.Config.Baud = origbaud
}
func CheetahReset(serialport string, reactor Reactor) {
	cfg := &serial.Config{Name: serialport, Baud: 2400, ReadTimeout: 0}
	ser, err := serial.OpenPort(cfg)
	if err != nil {
		logger.Error(err.Error())
	}
	defer ser.Close()
	bs := make([]byte, 4096)
	_, err1 := ser.Read(bs)
	if err1 != nil {
		logger.Error(err1.Error())
	}
	reactor.Pause(reactor.Monotonic() + 0.100)
	reactor.Pause(reactor.Monotonic() + 0.100)
	reactor.Pause(reactor.Monotonic() + 0.100)
	reactor.Pause(reactor.Monotonic() + 0.100)
	reactor.Pause(reactor.Monotonic() + 0.100)
	reactor.Pause(reactor.Monotonic() + 0.100)
}
func ArduinoReset(serialport string, reactor Reactor) {
	cfg := &serial.Config{Name: serialport, Baud: 2400, ReadTimeout: 0}
	ser, err := serial.OpenPort(cfg)
	if err != nil {
		logger.Error(err.Error())
	}
	defer ser.Close()
	bs := make([]byte, 4096)
	_, err1 := ser.Read(bs)
	if err1 != nil {
		logger.Error(err1.Error())
	}
	reactor.Pause(reactor.Monotonic() + 0.100)
	reactor.Pause(reactor.Monotonic() + 0.100)
	reactor.Pause(reactor.Monotonic() + 0.100)
}
