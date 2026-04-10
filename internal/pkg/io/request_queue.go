package io

import "math"

type ActionCallback func(float64, float64) (string, float64)

type RequestQueue struct {
	callback         ActionCallback
	rqueue           [][]float64
	nextMinFlushTime float64
}

func NewRequestQueue(callback ActionCallback) *RequestQueue {
	self := &RequestQueue{}
	self.callback = callback
	self.rqueue = [][]float64{}
	self.nextMinFlushTime = 0.0
	return self
}

func (self *RequestQueue) Flush(printTime float64, noteActivity func(float64)) {
	for len(self.rqueue) > 0 {
		nextTime := math.Max(self.rqueue[0][0], self.nextMinFlushTime)
		if nextTime > printTime {
			return
		}

		pos := 0
		for pos+1 < len(self.rqueue) && self.rqueue[pos+1][0] <= nextTime {
			pos++
		}
		reqVal := self.rqueue[pos][1]
		action, minWait := self.callback(nextTime, reqVal)
		if action != "" {
			if action == "discard" {
				self.rqueue = self.rqueue[pos+1:]
				continue
			}
			if action == "delay" {
				pos--
			}
		}

		self.rqueue = self.rqueue[pos+1:]
		self.nextMinFlushTime = nextTime + math.Max(minWait, 0.01)
		if noteActivity != nil {
			noteActivity(self.nextMinFlushTime)
		}
	}
}

func (self *RequestQueue) QueueRequest(printTime float64, value float64, noteActivity func(float64)) {
	self.rqueue = append(self.rqueue, []float64{printTime, value})
	if noteActivity != nil {
		noteActivity(printTime)
	}
}

func (self *RequestQueue) SendAsyncRequest(value float64, printTime float64) {
	for {
		nextTime := math.Max(printTime, self.nextMinFlushTime)
		action, minWait := self.callback(nextTime, value)
		if action != "" && action == "discard" {
			break
		}
		self.nextMinFlushTime = nextTime + math.Max(minWait, 0.01)
		if action != "delay" {
			break
		}
	}
}