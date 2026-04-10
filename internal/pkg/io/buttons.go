package io

import "math"

type ButtonPinConfig struct {
	Invert bool
}

type DigitalButtonState struct {
	pinCount    int
	invertMask  int
	lastButton  int
	ackCount    int
	callbacks   []digitalButtonCallback
}

type digitalButtonCallback struct {
	mask     int
	shift    int
	callback func(float64, int)
}

func NewDigitalButtonState() *DigitalButtonState {
	return &DigitalButtonState{callbacks: []digitalButtonCallback{}}
}

func (self *DigitalButtonState) AddPins(pins []ButtonPinConfig, callback func(float64, int)) {
	mask := 0
	shift := self.pinCount
	for _, pin := range pins {
		if pin.Invert {
			self.invertMask |= 1 << self.pinCount
		}
		mask |= 1 << self.pinCount
		self.pinCount++
	}
	self.callbacks = append(self.callbacks, digitalButtonCallback{mask: mask, shift: shift, callback: callback})
}

func (self *DigitalButtonState) PinCount() int {
	return self.pinCount
}

func (self *DigitalButtonState) InvertMask() int {
	return self.invertMask
}

func (self *DigitalButtonState) HandleButtonsState(msgAckCount int64, buttons []int,
	sendAck func(int), schedule func(func(float64))) {
	ackCount := self.ackCount
	ackDiff := (ackCount - int(msgAckCount)) & 0xff
	if ackDiff&0x80 != 0 {
		ackDiff -= 0x100
	}
	fullAckCount := ackCount - ackDiff
	newCount := fullAckCount + len(buttons) - self.ackCount
	if newCount <= 0 {
		return
	}
	newButtons := buttons[len(buttons)-newCount:]
	sendAck(newCount)
	self.ackCount += newCount
	for _, newButton := range newButtons {
		buttonValue := newButton
		schedule(func(eventtime float64) {
			self.HandleButton(eventtime, buttonValue)
		})
	}
}

func (self *DigitalButtonState) HandleButton(eventtime float64, button int) {
	button ^= self.invertMask
	changed := button ^ self.lastButton
	for _, item := range self.callbacks {
		if changed&item.mask != 0 {
			item.callback(eventtime, (button&item.mask)>>item.shift)
		}
	}
	self.lastButton = button
}

type adcButtonCallback struct {
	minValue float64
	maxValue float64
	callback func(float64, bool)
}

type ADCButtonState struct {
	buttons          []adcButtonCallback
	lastButton       int
	lastPressed      int
	lastDebounceTime float64
	pullup           float64
	minValue         float64
	maxValue         float64
	debounceTime     float64
}

func NewADCButtonState(pullup float64, debounceTime float64) *ADCButtonState {
	return &ADCButtonState{
		buttons:      []adcButtonCallback{},
		lastButton:   -1,
		lastPressed:  -1,
		pullup:       pullup,
		minValue:     999999999999.9,
		maxValue:     0.,
		debounceTime: debounceTime,
	}
}

func (self *ADCButtonState) AddButton(minValue, maxValue float64, callback func(float64, bool)) {
	self.minValue = math.Min(self.minValue, minValue)
	self.maxValue = math.Max(self.maxValue, maxValue)
	self.buttons = append(self.buttons, adcButtonCallback{minValue: minValue, maxValue: maxValue, callback: callback})
}

func (self *ADCButtonState) ADCCallback(readTime float64, readValue float64, schedule func(func(float64))) {
	adc := math.Max(.00001, math.Min(.99999, readValue))
	value := self.pullup * adc / (1.0 - adc)

	buttonIndex := -1
	if self.minValue <= value && value <= self.maxValue {
		for index, item := range self.buttons {
			if item.minValue < value && value < item.maxValue {
				buttonIndex = index
				break
			}
		}
	}

	if buttonIndex != self.lastButton {
		self.lastDebounceTime = readTime
	}

	if readTime-self.lastDebounceTime > self.debounceTime &&
		self.lastButton == buttonIndex && self.lastPressed != buttonIndex {
		if self.lastPressed >= 0 {
			pressedIndex := self.lastPressed
			schedule(func(eventtime float64) {
				self.buttons[pressedIndex].callback(eventtime, false)
			})
			self.lastPressed = -1
		}
		if buttonIndex >= 0 {
			pressedIndex := buttonIndex
			schedule(func(eventtime float64) {
				self.buttons[pressedIndex].callback(eventtime, true)
			})
			self.lastPressed = buttonIndex
		}
	}

	self.lastButton = buttonIndex
}

type RotaryEncoder struct {
	cwCallback   func(float64)
	ccwCallback  func(float64)
	encoderState int
	states       [][]int
	rDirCW       int
	rDirCCW      int
	rDirMask     int
}

func (self *RotaryEncoder) Encoder_callback(eventtime float64, state int) {
	es := self.states[self.encoderState&0xf][state&0x3]
	self.encoderState = es
	if es&self.rDirMask == self.rDirCW {
		self.cwCallback(eventtime)
	} else if es&self.rDirMask == self.rDirCCW {
		self.ccwCallback(eventtime)
	}
}

func NewFullStepRotaryEncoder(cwCallback, ccwCallback func(float64)) *RotaryEncoder {
	const (
		rStart    = 0x0
		rDirCW    = 0x10
		rDirCCW   = 0x20
		rDirMask  = 0x30
		rCWFinal  = 0x1
		rCWBegin  = 0x2
		rCWNext   = 0x3
		rCCWBegin = 0x4
		rCCWFinal = 0x5
		rCCWNext  = 0x6
	)
	return &RotaryEncoder{
		cwCallback:   cwCallback,
		ccwCallback:  ccwCallback,
		encoderState: rStart,
		rDirCW:       rDirCW,
		rDirCCW:      rDirCCW,
		rDirMask:     rDirMask,
		states: [][]int{
			{rStart, rCWBegin, rCCWBegin, rStart},
			{rCWNext, rStart, rCWFinal, rStart | rDirCW},
			{rCWNext, rCWBegin, rStart, rStart},
			{rCWNext, rCWBegin, rCWFinal, rStart},
			{rCCWNext, rStart, rCCWBegin, rStart},
			{rCCWNext, rCCWFinal, rStart, rStart | rDirCCW},
			{rCCWNext, rCCWFinal, rCCWBegin, rStart},
		},
	}
}

func NewHalfStepRotaryEncoder(cwCallback, ccwCallback func(float64)) *RotaryEncoder {
	const (
		rStart      = 0x0
		rDirCW      = 0x10
		rDirCCW     = 0x20
		rDirMask    = 0x30
		rCCWBegin   = 0x1
		rCWBegin    = 0x2
		rStartM     = 0x3
		rCWBeginM   = 0x4
		rCCWBeginM  = 0x5
	)
	return &RotaryEncoder{
		cwCallback:   cwCallback,
		ccwCallback:  ccwCallback,
		encoderState: rStart,
		rDirCW:       rDirCW,
		rDirCCW:      rDirCCW,
		rDirMask:     rDirMask,
		states: [][]int{
			{rStartM, rCWBegin, rCCWBegin, rStart},
			{rStartM | rDirCCW, rStart, rCCWBegin, rStart},
			{rStartM | rDirCW, rCWBegin, rStart, rStart},
			{rStartM, rCCWBeginM, rCWBeginM, rStart},
			{rStartM, rStartM, rCWBeginM, rStart | rDirCW},
			{rStartM, rCCWBeginM, rStartM, rStart | rDirCCW},
		},
	}
}