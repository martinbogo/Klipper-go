package main

import (
	"errors"
	"testing"
)

type scriptedReadStep struct {
	data []byte
	err  error
}

type scriptedReader struct {
	steps []scriptedReadStep
	idx   int
}

func (r *scriptedReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.steps) {
		return 0, nil
	}
	step := r.steps[r.idx]
	r.idx++
	n := copy(p, step.data)
	return n, step.err
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestDrainPendingInputStopsAfterQuietPoll(t *testing.T) {
	reader := &scriptedReader{steps: []scriptedReadStep{{data: []byte{1, 2, 3}}, {}}}

	discarded, polls, quiet, err := drainPendingInput(reader, 1, 4)
	if err != nil {
		t.Fatalf("drainPendingInput returned error: %v", err)
	}
	if discarded != 3 {
		t.Fatalf("discarded = %d, want 3", discarded)
	}
	if polls != 2 {
		t.Fatalf("polls = %d, want 2", polls)
	}
	if !quiet {
		t.Fatalf("quiet = false, want true")
	}
}

func TestDrainPendingInputHonorsPollLimitWhenTrafficStaysActive(t *testing.T) {
	reader := &scriptedReader{steps: []scriptedReadStep{{data: []byte{1}}, {data: []byte{2}}, {data: []byte{3}}, {data: []byte{4}}}}

	discarded, polls, quiet, err := drainPendingInput(reader, 1, 3)
	if err != nil {
		t.Fatalf("drainPendingInput returned error: %v", err)
	}
	if discarded != 3 {
		t.Fatalf("discarded = %d, want 3", discarded)
	}
	if polls != 3 {
		t.Fatalf("polls = %d, want 3", polls)
	}
	if quiet {
		t.Fatalf("quiet = true, want false")
	}
}

func TestDrainPendingInputTreatsTimeoutAsQuietPoll(t *testing.T) {
	reader := &scriptedReader{steps: []scriptedReadStep{{err: timeoutError{}}}}

	discarded, polls, quiet, err := drainPendingInput(reader, 1, 2)
	if err != nil {
		t.Fatalf("drainPendingInput returned error: %v", err)
	}
	if discarded != 0 {
		t.Fatalf("discarded = %d, want 0", discarded)
	}
	if polls != 1 {
		t.Fatalf("polls = %d, want 1", polls)
	}
	if !quiet {
		t.Fatalf("quiet = false, want true")
	}
}

func TestDrainPendingInputReturnsReadError(t *testing.T) {
	wantErr := errors.New("boom")
	reader := &scriptedReader{steps: []scriptedReadStep{{data: []byte{1, 2}}, {err: wantErr}}}

	discarded, polls, quiet, err := drainPendingInput(reader, 1, 4)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if discarded != 2 {
		t.Fatalf("discarded = %d, want 2", discarded)
	}
	if polls != 2 {
		t.Fatalf("polls = %d, want 2", polls)
	}
	if quiet {
		t.Fatalf("quiet = true, want false")
	}
}

func TestTimeoutErrorSatisfiesTemporaryContract(t *testing.T) {
	var err error = timeoutError{}
	if !err.(interface{ Timeout() bool }).Timeout() {
		t.Fatalf("timeout error should report Timeout() = true")
	}
	if !err.(interface{ Temporary() bool }).Temporary() {
		t.Fatalf("timeout error should report Temporary() = true")
	}
}
