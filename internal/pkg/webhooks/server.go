package webhooks

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"syscall"

	"goklipper/common/constants"
	kerror "goklipper/common/errors"
	"goklipper/common/logger"
	"goklipper/common/utils/sys"
	"goklipper/internal/pkg/queue"
	reactorpkg "goklipper/internal/pkg/reactor"
	"goklipper/internal/pkg/util"

	uuid "github.com/satori/go.uuid"
)

type RequestEnvelope interface {
	RuntimeRequest
	MethodName() string
	Finish() map[string]interface{}
	SetError(error)
}

type RequestFactory func(client *ClientConnection, raw string) (RequestEnvelope, error)

type ClientInfoHook func(client *ClientConnection, clientInfo string)

type ServerSocket struct {
	reactor        reactorpkg.IReactor
	sock           net.Listener
	fdHandle       *reactorpkg.ReactorFileHandler
	clients        map[string]*ClientConnection
	registry       *EndpointRegistry
	requestFactory RequestFactory
	clientInfoHook ClientInfoHook
}

func NewServerSocket(
	reactor reactorpkg.IReactor,
	serverAddress string,
	registry *EndpointRegistry,
	requestFactory RequestFactory,
	clientInfoHook ClientInfoHook,
) *ServerSocket {
	server := &ServerSocket{
		reactor:        reactor,
		clients:        map[string]*ClientConnection{},
		registry:       registry,
		requestFactory: requestFactory,
		clientInfoHook: clientInfoHook,
	}
	if serverAddress == "" {
		return server
	}

	if err := RemoveSocketFile(serverAddress); err != nil {
		logger.Error(err.Error())
	}
	sa, err := net.ResolveUnixAddr("unix", serverAddress)
	if err != nil {
		logger.Error(err.Error())
		return server
	}
	listener, err := net.ListenUnix("unix", sa)
	if err != nil {
		logger.Error(err.Error())
		return server
	}
	server.sock = listener

	rawConn, err := listener.SyscallConn()
	if err != nil {
		logger.Error(err.Error())
		return server
	}
	var fd int
	if err = rawConn.Control(func(f uintptr) {
		fd = int(f)
	}); err != nil {
		logger.Error(err.Error())
		return server
	}
	if err = syscall.SetNonblock(fd, true); err != nil {
		logger.Error(err.Error())
	}
	server.fdHandle = reactor.Register_fd(fd, server.handleAccept, nil)
	return server
}

func (s *ServerSocket) handleAccept(eventtime float64) interface{} {
	if s.sock == nil {
		return nil
	}
	logger.Debug("k3c accept new socket")
	sock, err := s.sock.Accept()
	if err != nil {
		logger.Error(err.Error())
		return "exit"
	}
	if err = syscall.SetNonblock(util.Fileno(sock), true); err != nil {
		logger.Error(err.Error())
	}
	client := NewClientConnection(s, sock)
	s.clients[client.uid] = client
	return nil
}

func (s *ServerSocket) HandleDisconnect(params []interface{}) error {
	for _, client := range s.clients {
		if err := client.Close(); err != nil {
			logger.Error(err.Error())
		}
	}
	if s.sock != nil {
		s.reactor.Unregister_fd(s.fdHandle)
		if err := s.sock.Close(); err != nil {
			logger.Error(err.Error())
		}
	}
	return nil
}

func (s *ServerSocket) HandleShutdown(params []interface{}) error {
	for _, client := range s.clients {
		client.DumpRequestLog()
	}
	return nil
}

func (s *ServerSocket) Stats(eventtime float64) (bool, string) {
	for _, client := range s.clients {
		if client.isBlocking {
			client.blockingCount -= 1
			if client.blockingCount < 0 {
				logger.Infof("Closing unresponsive client %s", client.uid)
				_ = client.Close()
			}
		}
	}
	return true, ""
}

func (s *ServerSocket) popClient(clientID string) {
	delete(s.clients, clientID)
}

type ClientConnection struct {
	reactor       reactorpkg.IReactor
	server        *ServerSocket
	uid           string
	fdHandle      *reactorpkg.ReactorFileHandler
	partialData   []byte
	isBlocking    bool
	blockingCount int
	sock          net.Conn
	sendBuffer    []byte
	requestLog    *queue.Queue
	sendMutex     sync.Mutex
}

var _ RuntimeClient = (*ClientConnection)(nil)

func NewClientConnection(server *ServerSocket, sock net.Conn) *ClientConnection {
	client := &ClientConnection{
		reactor:       server.reactor,
		server:        server,
		uid:           uuid.NewV4().String(),
		sock:          sock,
		partialData:   []byte{},
		sendBuffer:    []byte{},
		isBlocking:    false,
		blockingCount: 0,
		requestLog:    queue.NewQueue(),
		sendMutex:     sync.Mutex{},
	}
	client.fdHandle = client.reactor.Register_fd(util.Fileno(sock), client.processReceived, client.doSend)
	client.SetClientInfo("?", "New connection")
	return client
}

func (c *ClientConnection) UID() string {
	return c.uid
}

type ClientRequest struct {
	eventtime float64
	request   string
}

func (c *ClientConnection) DumpRequestLog() {
	out := []string{}
	out = append(out, fmt.Sprintf("Dumping %d requests for client %s", c.requestLog.Len(), c.uid))
	for {
		if c.requestLog.Is_empty() {
			break
		}
		clientRequest := c.requestLog.Get_nowait().(ClientRequest)
		out = append(out, fmt.Sprintf("Received %f: %s", clientRequest.eventtime, clientRequest.request))
	}
	logger.Info(strings.Join(out, "\n"))
}

func (c *ClientConnection) SetClientInfo(clientInfo string, stateMsg string) {
	if stateMsg == "" {
		stateMsg = fmt.Sprintf("Client info %s", clientInfo)
	}
	logger.Infof("webhooks client %s: %s", c.uid, stateMsg)
	if c.server.clientInfoHook != nil {
		c.server.clientInfoHook(c, clientInfo)
	}
}

func (c *ClientConnection) Close() error {
	if c.fdHandle == nil {
		return nil
	}
	c.SetClientInfo("", "Disconnected")
	c.reactor.Unregister_fd(c.fdHandle)
	c.fdHandle = nil

	err := c.sock.Close()
	if err != nil {
		logger.Error(err.Error())
	}
	c.server.popClient(c.uid)
	return nil
}

func (c *ClientConnection) Is_closed() bool {
	return c.fdHandle == nil
}

func (c *ClientConnection) IsClosed() bool {
	return c.Is_closed()
}

func (c *ClientConnection) processReceived(eventtime float64) interface{} {
	bs := make([]byte, 4096)
	count, err := c.sock.Read(bs)
	if err != nil {
		if err != io.EOF {
			logger.Errorf("socket read error: %s", err.Error())
		}
		if err == syscall.EBADF || err == io.EOF {
			_ = c.Close()
			return nil
		}
		_ = c.Close()
		return err
	}

	var requests []string
	for i := 0; i < count; i++ {
		if bs[i] == 0x03 {
			requests = append(requests, string(c.partialData))
			c.partialData = []byte{}
			continue
		}
		c.partialData = append(c.partialData, bs[i])
	}
	for _, raw := range requests {
		c.requestLog.Put_nowait(ClientRequest{eventtime: eventtime, request: raw})
		request, reqErr := c.server.requestFactory(c, raw)
		if reqErr != nil || request == nil {
			logger.Error("webhooks: Error decoding Server Request ", raw)
			if reqErr != nil {
				logger.Error(reqErr.Error())
			}
			continue
		}
		c.reactor.Register_callback(func(argv interface{}) interface{} {
			c.processRequest(request)
			return argv.(float64)
		}, constants.NOW)
	}
	return nil
}

func (c *ClientConnection) processRequest(request RequestEnvelope) {
	handler := c.server.registry.Handler(request.MethodName())
	if handler == nil {
		logger.Error("unregistered for method:", request.MethodName())
		request.SetError(kerror.NewWebRequestError(kerror.MethodUnregisteredCode, "web request method: "+request.MethodName()+" unregistered"))
	} else {
		if _, err := c.processRequestSafe(handler, request); err != nil {
			request.SetError(err)
		}
	}

	result := request.Finish()
	if result == nil {
		return
	}
	c.Send(result)
}

func (c *ClientConnection) processRequestSafe(handler func(RuntimeRequest) (interface{}, error), request RuntimeRequest) (interface{}, error) {
	defer sys.CatchPanic()
	return handler(request)
}

func (c *ClientConnection) Send(data interface{}) {
	if data == nil {
		return
	}
	safeData := sanitizeJSONValue(data)
	jmsg, err := json.Marshal(safeData)
	if err != nil {
		logger.Error(err.Error())
		return
	}
	c.sendMutex.Lock()
	c.sendBuffer = append(jmsg, '\x03')
	c.sendMutex.Unlock()
	if !c.isBlocking {
		c.doSend(0)
	}
}

func sanitizeJSONValue(value interface{}) interface{} {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
		return v
	case float32:
		asFloat64 := float64(v)
		if math.IsNaN(asFloat64) || math.IsInf(asFloat64, 0) {
			return nil
		}
		return v
	case map[string]interface{}:
		sanitized := make(map[string]interface{}, len(v))
		for k, item := range v {
			sanitized[k] = sanitizeJSONValue(item)
		}
		return sanitized
	case []interface{}:
		sanitized := make([]interface{}, len(v))
		for i, item := range v {
			sanitized[i] = sanitizeJSONValue(item)
		}
		return sanitized
	default:
		return value
	}
}

func (c *ClientConnection) doSend(eventtime float64) interface{} {
	if c.fdHandle == nil {
		return nil
	}
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()
	sent, err := c.sock.Write(c.sendBuffer)
	if err != nil {
		logger.Error("webhooks: socket write error", c.uid, err.Error())
		if err == io.EOF {
			_ = c.Close()
			return nil
		}
		sent = 0
	}
	if sent < len(c.sendBuffer) {
		if !c.isBlocking {
			c.reactor.Set_fd_wake(c.fdHandle, false, true)
			c.isBlocking = true
			c.blockingCount = 5
		}
	} else if c.isBlocking {
		c.reactor.Set_fd_wake(c.fdHandle, true, false)
		c.isBlocking = false
	}
	c.sendBuffer = c.sendBuffer[sent:]
	return nil
}
