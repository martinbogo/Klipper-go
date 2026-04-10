package serialhdl

import (
	"errors"
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/reflects"
	"goklipper/common/utils/sys"
	"goklipper/internal/pkg/chelper"
	"goklipper/internal/pkg/msgproto"
	"goklipper/internal/pkg/util"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tarm/serial"
)

type SerialReader struct {
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
	last_notify_id        int64
	pending_notifications sync.Map
	queueLock             sync.Locker
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
func (self *SerialReader) bg_thread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for {
		if self.taskThread() {
			break
		}
	}
}
func (self *SerialReader) taskThread() bool {
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
		self.queueLock.Lock()
		if self.Serialqueue == nil {
			self.queueLock.Unlock()
			return true
		}
		chelper.Serialqueue_pull(self.Serialqueue, response)
		self.queueLock.Unlock()
		count := response.Len
		if count < 0 {
			time.Sleep(time.Second)
			continue
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
		hdl, ok := params["#name"]
		if ok {
			hdl1, ok1 := params["oid"]
			if ok1 {
				hdl = strings.Join([]string{hdl.(string), strconv.FormatInt(hdl1.(int64), 10)}, "")
			}
			self.lock.Lock()
			fn := self.handlers[hdl.(string)]
			self.lock.Unlock()
			var err error
			if fn != nil {
				err = fn.(func(map[string]interface{}) error)(params)
				if err != nil {
					logger.Errorf("%sException in serial callback", self.warn_prefix)
				}
			} else {
				logger.Debugf("%s Unhandled serial callback %s", self.warn_prefix, hdl.(string))
			}
		}
	}
}
func (self *SerialReader) _error(msg string, params map[string]interface{}) {
	panic(fmt.Sprintf("%s%s%s", self.warn_prefix, msg, params))
}
func (self *SerialReader) _get_identify_data(eventtime interface{}) interface{} {
	identify_data := []byte{}
	for {
		msg := fmt.Sprintf("identify offset=%d count=%d", len(identify_data), 40)
		logger.Infof("Identify Request: %s", msg)
		params, err := self.Send_with_response(msg, "identify_response")
		if err != nil {
			logger.Errorf("Identify Error: %v", err)
			return nil
		}
		logger.Infof("Identify Got Offset: %v", params["offset"])
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

func (self *SerialReader) _start_session(serial_dev SerialDeviceBase, serial_fd_type byte, client_id int) bool {
	self.serial_dev = serial_dev
	self.Serialqueue = chelper.Serialqueue_alloc(serial_dev.GetFd(), serial_fd_type, client_id)
	go self.bg_thread()
	go self.timeoutThread()
	completion := self.reactor.RegisterCallback(self._get_identify_data, constants.NOW)
	identify_data := completion.Wait(self.reactor.Monotonic()+5., nil)
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
		start_time = self.reactor.Monotonic()
		if start_time > start_time+90. {
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
		serialDev := NewSerialDev(serial_dev, cfg, reflects.GetPrivateFieldValue(serial_dev, "f").(*os.File))
		ret := self._start_session(serialDev, 'u', 0)
		if ret {
			break
		}
	}
}
func (self *SerialReader) Connect_remote(serialport string) {
	logger.Debugf("%sStarting remote connect %s", self.warn_prefix, serialport)
	var start_time float64
	for {
		start_time = self.reactor.Monotonic()
		if start_time > start_time+90. {
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
	chelper.Serialqueue_set_clock_est(self.Serialqueue, freq, conv_time, uint64(conv_clock), uint64(last_clock))
}
func (self *SerialReader) Disconnect() {
	if self.Serialqueue != nil {
		chelper.Serialqueue_exit(self.Serialqueue)
		if self.background_thread != nil {
			self.background_thread = nil
		}
		chelper.Serialqueue_free(self.Serialqueue)
		self.Serialqueue = nil
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
	if self.Serialqueue == nil {
		return ""
	}
	chelper.Serialqueue_get_stats(self.Serialqueue, self.stats_buf)
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
func (self *SerialReader) Raw_send(cmd []int, minclock, reqclock int64, cmd_queue interface{}) {
	chelper.Serialqueue_send(self.Serialqueue, cmd_queue, cmd, len(cmd), minclock, reqclock, 0)
}
func (self *SerialReader) Raw_send_wait_ack(cmd []int, minclock, reqclock int64, cmd_queue interface{}) map[string]interface{} {
	self.last_notify_id += 1
	nid := self.last_notify_id
	completion := self.reactor.NewCompletion()
	self.pending_notifications.Store(nid, completion)
	chelper.Serialqueue_send(self.Serialqueue, cmd_queue, cmd, len(cmd), minclock, reqclock, nid)
	w_time := self.reactor.Monotonic() + 2.0
	params := completion.Wait(w_time, nil)
	self.pending_notifications.Delete(nid)
	if params == nil {
		logger.Warnf("Serial connection packet dropped/timeout %d", nid)
		return nil
	}
	return params.(map[string]interface{})
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
func (self *SerialReader) _handle_unknown_init(params map[string]interface{}) error {
	var id int
	switch v := params["#msgid"].(type) {
	case int64:
		id = int(v)
	case int:
		id = v
	case float64:
		id = int(v)
	}
	if id != 77 && id != 70 {
		logger.Warnf("%sUnknown INIT message %d (len %T)", self.warn_prefix, id, params["#msg"])
	}
	return nil
}
func (self *SerialReader) Handle_unknown(params map[string]interface{}) error {
	var id int
	switch v := params["#msgid"].(type) {
	case int64:
		id = int(v)
	case int:
		id = v
	case float64:
		id = int(v)
	}
	if id != 77 && id != 70 {
		logger.Warnf("%sUnknown MSG type %d: %s", self.warn_prefix, id, params["#msg"])
	}
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

type SerialDev struct {
	Port   *serial.Port
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
func NewSerialDev(port *serial.Port, config *serial.Config, file *os.File) *SerialDev {
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
}

func NewSerialRetryCommand(serial *SerialReader, name string, oid interface{}) *SerialRetryCommand {
	self := SerialRetryCommand{}
	self.serial = serial
	self.name = name
	self.oid = oid
	self.last_params = nil
	self.serial.Register_response(self.handle_callback, name, oid)
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
		lastCmdIndex := len(cmds) - 1
		if lastCmdIndex < 0 {
			return map[string]interface{}{}, nil
		}
		for i, cmd := range cmds {
			if i == lastCmdIndex {
				continue
			}
			self.serial.Raw_send(cmd.([]int), minclock, reqclock, cmd_queue)
		}
		self.serial.Raw_send_wait_ack(cmds[lastCmdIndex].([]int), minclock, reqclock, cmd_queue)
		params := self.last_params
		if params != nil {
			self.serial.Register_response(nil, self.name, self.oid)
			return params, nil
		}
		if retries <= 0 {
			self.serial.Register_response(nil, self.name, self.oid)
			return nil, errors.New(fmt.Sprintf("Unable to obtain '%s' response", self.name))
		}
		reactor := self.serial.reactor
		reactor.Pause(reactor.Monotonic() + retry_delay)
		retries -= 1
		retry_delay *= 2.
	}
}

type RetryAsyncCommand struct {
	Completion     Completion
	Reactor        Reactor
	Serial         *SerialReader
	Oid            int
	Min_query_time float64
	Name           string
	need_response  bool
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
	self.Serial.Register_response(self.Handle_callback, name, oid)
}
func (self *RetryAsyncCommand) Handle_callback(params map[string]interface{}) error {
	send_time := chelper.CdoubleTofloat64(params["#sent_time"])
	if self.need_response && send_time >= self.Min_query_time {
		self.need_response = false
		self.Reactor.AsyncComplete(self.Completion, params)
	}
	return nil
}
func (self *RetryAsyncCommand) get_response(cmds []interface{}, cmd_queue interface{}, minclock, reqclock int64) (map[string]interface{}, error) {
	cmd := cmds[0]
	self.Serial.Raw_send_wait_ack(cmd.([]int), minclock, reqclock, cmd_queue)
	self.Min_query_time = 0
	first_query_time := self.Reactor.Monotonic()
	query_time := first_query_time
	for {
		params := self.Completion.Wait(query_time+RETRY_TIME, nil)
		if params != nil {
			self.Serial.Register_response(nil, self.Name, self.Oid)
			return params.(map[string]interface{}), nil
		}
		query_time = self.Reactor.Monotonic()
		if query_time > first_query_time+TIMEOUT_TIME {
			self.Serial.Register_response(nil, self.Name, self.Oid)
			panic(fmt.Sprintf("Timeout on wait for '%s' response '%d'", self.Name, self.Oid))
		}
		self.Serial.Raw_send(cmd.([]int), minclock, minclock, cmd_queue)
	}
}

type RetryCommand interface {
	get_response(cmds []interface{}, cmd_queue interface{}, minclock, reqclock int64) (map[string]interface{}, error)
}

func NewRetryAsyncCommand(serial *SerialReader, name string, oid int) *RetryAsyncCommand {
	ob := RetryAsyncCommand{}
	ob.__init__(serial, name, oid)
	return &ob
}

type CommandQueryWrapper struct {
	Xmit_helper bool
	Cmd_queue   interface{}
	Error       interface{}
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
	self.Error = error
	self.Xmit_helper = is_async
	if cmd_queue == nil {
		cmd_queue = serial.Get_default_command_queue()
	}
	self.Cmd_queue = cmd_queue
	return self
}
func (self *CommandQueryWrapper) Do_send(cmds [][]int, minclock, reqclock int64) interface{} {
	var xh interface{}
	if self.Xmit_helper {
		xh = NewRetryAsyncCommand(self.Serial, self.Response, self.Oid)
	} else {
		xh = NewSerialRetryCommand(self.Serial, self.Response, self.Oid)
	}
	if reqclock < minclock {
		reqclock = minclock
	}
	cmd := []interface{}{}
	for _, c := range cmds {
		cmd = append(cmd, c)
	}
	res, _ := xh.(RetryCommand).get_response(cmd, self.Cmd_queue, minclock, reqclock)
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

func Stk500v2Leave(ser *SerialDev, reactor Reactor) {
	log.Print("Starting stk500v2 leave programmer sequence")
	util.Clear_hupcl(ser.file.Fd())
	origbaud := ser.Config.Baud
	ser.Config.Baud = 2400
	bs := make([]byte, 4096)
	_, err := ser.Port.Read(bs)
	if err != nil {
		logger.Error(err.Error())
	}
	ser.Config.Baud = 115200
	reactor.Pause(reactor.Monotonic() + 0.100)
	ser.Port.Read(bs)
	writeData := []byte("\x1b\x01\x00\x01\x0e\x11\x04")
	ser.Port.Write(writeData)
	reactor.Pause(reactor.Monotonic() + 0.050)
	count, err1 := ser.Port.Read(bs)
	if err1 != nil {
		logger.Error(err1.Error())
	}
	logger.Infof("Got %X from stk500v2", string(bs[:count]))
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
