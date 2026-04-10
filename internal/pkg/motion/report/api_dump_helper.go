package report

import (
	"goklipper/common/constants"
	"goklipper/common/logger"
)

type APIDumpReactor interface {
	Monotonic() float64
	RegisterTimer(callback func(float64) float64, waketime float64) interface{}
	UnregisterTimer(timer interface{})
}

type APIDumpClient interface {
	IsClosed() bool
	Send(msg map[string]interface{})
}

type APIDumpHelper struct {
	reactor        APIDumpReactor
	dataCB         func(eventtime float64) map[string]interface{}
	startstopCB    func(isStart bool)
	updateInterval float64
	isStarted      bool
	updateTimer    interface{}
	clients        map[APIDumpClient]map[string]interface{}
}

func NewAPIDumpHelper(reactor APIDumpReactor, dataCB func(float64) map[string]interface{}, startstopCB func(bool), updateInterval float64) *APIDumpHelper {
	if startstopCB == nil {
		startstopCB = func(bool) {}
	}
	return &APIDumpHelper{
		reactor:        reactor,
		dataCB:         dataCB,
		startstopCB:    startstopCB,
		updateInterval: updateInterval,
		clients:        map[APIDumpClient]map[string]interface{}{},
	}
}

func (self *APIDumpHelper) AddClient(client APIDumpClient, template map[string]interface{}) {
	self.clients[client] = template
	self.start()
}

func (self *APIDumpHelper) AddInternalClient() *InternalDumpClient {
	client := NewInternalDumpClient()
	self.clients[client] = map[string]interface{}{}
	self.start()
	return client
}

func (self *APIDumpHelper) stop() float64 {
	self.clients = map[APIDumpClient]map[string]interface{}{}
	self.reactor.UnregisterTimer(self.updateTimer)
	self.updateTimer = nil
	if !self.isStarted {
		return constants.NEVER
	}
	self.callStartstop(false, "stop")
	self.isStarted = false
	if len(self.clients) > 0 {
		self.start()
	}
	return constants.NEVER
}

func (self *APIDumpHelper) start() {
	if self.isStarted {
		return
	}
	self.isStarted = true
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("API Dump Helper start callback error: %v", r)
				self.isStarted = false
				self.clients = map[APIDumpClient]map[string]interface{}{}
				panic(r)
			}
		}()
		self.startstopCB(true)
	}()
	waketime := self.reactor.Monotonic() + self.updateInterval
	self.updateTimer = self.reactor.RegisterTimer(self.update, waketime)
}

func (self *APIDumpHelper) update(eventtime float64) float64 {
	msg, updated, recovered := self.callDataCB(eventtime)
	if recovered != nil {
		logger.Errorf("API Dump Helper data callback error")
		return updated
	}
	if msg == nil {
		return eventtime + self.updateInterval
	}
	for client, template := range self.clients {
		if client.IsClosed() {
			delete(self.clients, client)
			if len(self.clients) == 0 {
				return self.stop()
			}
			continue
		}
		payload := map[string]interface{}{}
		for key, value := range template {
			payload[key] = value
		}
		payload["params"] = msg
		client.Send(payload)
	}
	return eventtime + self.updateInterval
}

func (self *APIDumpHelper) callDataCB(eventtime float64) (msg interface{}, updated float64, recovered interface{}) {
	defer func() {
		if recovered = recover(); recovered != nil {
			logger.Error("API Dump Helper data callback error", recovered)
			updated = self.stop()
		}
	}()
	msg = self.dataCB(eventtime)
	return
}

func (self *APIDumpHelper) callStartstop(isStart bool, phase string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("API Dump Helper %s callback error: %v", phase, r)
		}
		self.clients = map[APIDumpClient]map[string]interface{}{}
	}()
	self.startstopCB(isStart)
}

type InternalDumpClient struct {
	msgs   []map[string]map[string]interface{}
	isDone bool
}

func NewInternalDumpClient() *InternalDumpClient {
	return &InternalDumpClient{msgs: []map[string]map[string]interface{}{}}
}

func (self *InternalDumpClient) Get_messages() []map[string]map[string]interface{} {
	return self.msgs
}

func (self *InternalDumpClient) Finalize() {
	self.isDone = true
}

func (self *InternalDumpClient) IsClosed() bool {
	return self.isDone
}

func (self *InternalDumpClient) Send(msg map[string]interface{}) {
	formatted := map[string]map[string]interface{}{}
	for key, value := range msg {
		formatted[key] = value.(map[string]interface{})
	}
	self.msgs = append(self.msgs, formatted)
	if len(self.msgs) >= 10000 {
		self.Finalize()
	}
}
