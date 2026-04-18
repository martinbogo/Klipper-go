package print

import (
	"fmt"
	"strings"
	"sync"
)

type LeviQ3CommandState struct {
	mu           sync.Mutex
	open         bool
	cancelled    bool
	cancelReason string
}

func NewLeviQ3CommandState() *LeviQ3CommandState {
	return &LeviQ3CommandState{}
}

func (state *LeviQ3CommandState) Begin() {
	if state == nil {
		return
	}
	state.mu.Lock()
	state.open = true
	state.cancelled = false
	state.cancelReason = ""
	state.mu.Unlock()
}

func (state *LeviQ3CommandState) Finish() {
	if state == nil {
		return
	}
	state.mu.Lock()
	state.open = false
	state.mu.Unlock()
}

func (state *LeviQ3CommandState) IsOpen() bool {
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.open
}

func (state *LeviQ3CommandState) NoteCancellation(reason string) string {
	if state == nil {
		return normalizeLeviQ3CancellationReason(reason)
	}
	normalized := normalizeLeviQ3CancellationReason(reason)
	state.mu.Lock()
	state.cancelled = true
	state.cancelReason = normalized
	state.mu.Unlock()
	return normalized
}

func (state *LeviQ3CommandState) CancelledState() (bool, string) {
	if state == nil {
		return false, ""
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.cancelled, state.cancelReason
}

func (state *LeviQ3CommandState) EnsureActive(stage string, printerShutdown bool) error {
	if printerShutdown {
		return fmt.Errorf("%s: printer shutdown", stage)
	}
	if cancelled, reason := state.CancelledState(); cancelled {
		if stage == "" {
			return fmt.Errorf("%s", reason)
		}
		return fmt.Errorf("%s: %s", stage, reason)
	}
	return nil
}

func normalizeLeviQ3CancellationReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "leviq3 cancelled"
	}
	return reason
}