package motion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type parityFixtureMove struct {
	StartPos []float64 `json:"start_pos"`
	EndPos   []float64 `json:"end_pos"`
	Speed    float64   `json:"speed"`
}

type parityFixtureMoveConfig struct {
	MaxAccel          float64 `json:"max_accel"`
	JunctionDeviation float64 `json:"junction_deviation"`
	MaxVelocity       float64 `json:"max_velocity"`
	MaxAccelToDecel   float64 `json:"max_accel_to_decel"`
}

type parityFixtureMoveResult struct {
	StartV  float64 `json:"start_v"`
	CruiseV float64 `json:"cruise_v"`
	EndV    float64 `json:"end_v"`
	AccelT  float64 `json:"accel_t"`
	CruiseT float64 `json:"cruise_t"`
	DecelT  float64 `json:"decel_t"`
}

type parityPlannerProfileFixture struct {
	Name              string                    `json:"name"`
	Config            parityFixtureMoveConfig   `json:"config"`
	Moves             []parityFixtureMove       `json:"moves"`
	JunctionResult    float64                   `json:"junction_result"`
	ExpectedProcessed []parityFixtureMoveResult `json:"expected_processed"`
}

type parityQueueTransitionFixture struct {
	Name                      string                  `json:"name"`
	Config                    parityFixtureMoveConfig `json:"config"`
	Moves                     []parityFixtureMove     `json:"moves"`
	JunctionResult            float64                 `json:"junction_result"`
	ExpectedLazyFlushRequests []bool                  `json:"expected_lazy_flush_requests"`
	ExpectedBatchSizes        []int                   `json:"expected_batch_sizes"`
	ExpectedRemainingQueueLen int                     `json:"expected_remaining_queue_len"`
}

type parityFlushConfigFixture struct {
	BufferTimeLow         float64 `json:"buffer_time_low"`
	BgFlushLowTime        float64 `json:"bg_flush_low_time"`
	BgFlushHighTime       float64 `json:"bg_flush_high_time"`
	BgFlushSgLowTime      float64 `json:"bg_flush_sg_low_time"`
	BgFlushSgHighTime     float64 `json:"bg_flush_sg_high_time"`
	BgFlushBatchTime      float64 `json:"bg_flush_batch_time"`
	BgFlushExtraTime      float64 `json:"bg_flush_extra_time"`
	StepcompressFlushTime float64 `json:"stepcompress_flush_time"`
}

type parityFlushStateFixture struct {
	PrintTime           float64 `json:"print_time"`
	LastFlushTime       float64 `json:"last_flush_time"`
	LastStepGenTime     float64 `json:"last_step_gen_time"`
	NeedFlushTime       float64 `json:"need_flush_time"`
	NeedStepGenTime     float64 `json:"need_step_gen_time"`
	SpecialQueuingState string  `json:"special_queuing_state"`
	KinFlushDelay       float64 `json:"kin_flush_delay"`
}

type parityFlushExpectedFixture struct {
	ShouldFlushLookahead bool      `json:"should_flush_lookahead"`
	AdvanceFlushTimes    []float64 `json:"advance_flush_times"`
	NextWakeTime         float64   `json:"next_wake_time"`
	ReturnNever          bool      `json:"return_never"`
	KickFlushTimer       bool      `json:"kick_flush_timer"`
}

type parityFlushWakeupFixture struct {
	Name               string                     `json:"name"`
	Config             parityFlushConfigFixture   `json:"config"`
	Eventtime          float64                    `json:"eventtime"`
	EstimatedPrintTime float64                    `json:"estimated_print_time"`
	State              parityFlushStateFixture    `json:"state"`
	Expected           parityFlushExpectedFixture `json:"expected"`
}

type parityTrapQTimelineEntryFixture struct {
	PrintTime float64 `json:"print_time"`
	StartV    float64 `json:"start_v"`
	CruiseV   float64 `json:"cruise_v"`
	Accel     float64 `json:"accel"`
	MoveTime  float64 `json:"move_time"`
}

type parityTrapQTimelineFixture struct {
	Name           string                            `json:"name"`
	Config         parityFixtureMoveConfig           `json:"config"`
	Moves          []parityFixtureMove               `json:"moves"`
	JunctionResult float64                           `json:"junction_result"`
	StartPrintTime float64                           `json:"start_print_time"`
	Expected       []parityTrapQTimelineEntryFixture `json:"expected"`
}

type parityFixtures struct {
	PlannerProfiles  []parityPlannerProfileFixture  `json:"planner_profiles"`
	QueueTransitions []parityQueueTransitionFixture `json:"queue_transitions"`
	FlushWakeups     []parityFlushWakeupFixture     `json:"flush_wakeups"`
	TrapQTimelines   []parityTrapQTimelineFixture   `json:"trapq_timelines"`
}

type parityFixtureProcessor struct {
	batches [][]*Move
}

func (self *parityFixtureProcessor) Process_moves(moves []*Move) {
	self.batches = append(self.batches, append([]*Move(nil), moves...))
}

func loadParityFixtures(t *testing.T) parityFixtures {
	t.Helper()
	path := filepath.Join("testdata", "parity_fixtures.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read parity fixtures: %v", err)
	}
	var fixtures parityFixtures
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("parse parity fixtures: %v", err)
	}
	return fixtures
}

func toMoveConfig(config parityFixtureMoveConfig) MoveConfig {
	return MoveConfig{
		Max_accel:          config.MaxAccel,
		Junction_deviation: config.JunctionDeviation,
		Max_velocity:       config.MaxVelocity,
		Max_accel_to_decel: config.MaxAccelToDecel,
	}
}

func toFlushConfig(config parityFlushConfigFixture) ToolheadFlushConfig {
	return ToolheadFlushConfig{
		BufferTimeLow:         config.BufferTimeLow,
		BgFlushLowTime:        config.BgFlushLowTime,
		BgFlushHighTime:       config.BgFlushHighTime,
		BgFlushSgLowTime:      config.BgFlushSgLowTime,
		BgFlushSgHighTime:     config.BgFlushSgHighTime,
		BgFlushBatchTime:      config.BgFlushBatchTime,
		BgFlushExtraTime:      config.BgFlushExtraTime,
		StepcompressFlushTime: config.StepcompressFlushTime,
	}
}

func toFlushState(state parityFlushStateFixture) ToolheadFlushHandlerState {
	return ToolheadFlushHandlerState{
		PrintTime:           state.PrintTime,
		LastFlushTime:       state.LastFlushTime,
		LastStepGenTime:     state.LastStepGenTime,
		NeedFlushTime:       state.NeedFlushTime,
		NeedStepGenTime:     state.NeedStepGenTime,
		SpecialQueuingState: state.SpecialQueuingState,
		KinFlushDelay:       state.KinFlushDelay,
	}
}

func assertParityFloat(t *testing.T, fixtureName string, field string, got float64, want float64) {
	t.Helper()
	if !almostEqualFloat64(got, want) {
		t.Fatalf("%s: expected %s=%v, got %v", fixtureName, field, want, got)
	}
}

func assertParityFloatSlice(t *testing.T, fixtureName string, field string, got []float64, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: expected %s len=%d, got %d", fixtureName, field, len(want), len(got))
	}
	for i := range want {
		assertParityFloat(t, fixtureName, field, got[i], want[i])
	}
}

func TestParityFixturePlannerProfiles(t *testing.T) {
	fixtures := loadParityFixtures(t)
	for _, fixture := range fixtures.PlannerProfiles {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			if len(fixture.Moves) != len(fixture.ExpectedProcessed) {
				t.Fatalf("fixture %s move/result length mismatch", fixture.Name)
			}
			processor := &parityFixtureProcessor{}
			queue := NewMoveQueue(processor)
			calculator := &fakeMoveJunctionCalculator{result: fixture.JunctionResult}
			config := toMoveConfig(fixture.Config)
			for _, moveFixture := range fixture.Moves {
				queue.Add_move(NewMove(config, moveFixture.StartPos, moveFixture.EndPos, moveFixture.Speed), calculator)
			}
			queue.Flush(false)
			if len(processor.batches) != 1 {
				t.Fatalf("%s: expected one processed batch, got %d", fixture.Name, len(processor.batches))
			}
			moves := processor.batches[0]
			if len(moves) != len(fixture.ExpectedProcessed) {
				t.Fatalf("%s: expected %d processed moves, got %d", fixture.Name, len(fixture.ExpectedProcessed), len(moves))
			}
			for i, expected := range fixture.ExpectedProcessed {
				assertParityFloat(t, fixture.Name, "start_v", moves[i].Start_v, expected.StartV)
				assertParityFloat(t, fixture.Name, "cruise_v", moves[i].Cruise_v, expected.CruiseV)
				assertParityFloat(t, fixture.Name, "end_v", moves[i].End_v, expected.EndV)
				assertParityFloat(t, fixture.Name, "accel_t", moves[i].Accel_t, expected.AccelT)
				assertParityFloat(t, fixture.Name, "cruise_t", moves[i].Cruise_t, expected.CruiseT)
				assertParityFloat(t, fixture.Name, "decel_t", moves[i].Decel_t, expected.DecelT)
			}
		})
	}
}

func TestParityFixtureQueueTransitions(t *testing.T) {
	fixtures := loadParityFixtures(t)
	for _, fixture := range fixtures.QueueTransitions {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			processor := &parityFixtureProcessor{}
			queue := NewMoveQueue(processor)
			calculator := &fakeMoveJunctionCalculator{result: fixture.JunctionResult}
			config := toMoveConfig(fixture.Config)
			requests := make([]bool, 0, len(fixture.Moves))
			for _, moveFixture := range fixture.Moves {
				requests = append(requests, queue.Add_move(NewMove(config, moveFixture.StartPos, moveFixture.EndPos, moveFixture.Speed), calculator))
			}
			if len(requests) != len(fixture.ExpectedLazyFlushRequests) {
				t.Fatalf("%s: expected %d lazy-flush results, got %d", fixture.Name, len(fixture.ExpectedLazyFlushRequests), len(requests))
			}
			for i := range fixture.ExpectedLazyFlushRequests {
				if requests[i] != fixture.ExpectedLazyFlushRequests[i] {
					t.Fatalf("%s: expected lazy-flush request %d to be %v, got %v", fixture.Name, i, fixture.ExpectedLazyFlushRequests[i], requests[i])
				}
			}
			queue.Flush(true)
			if len(processor.batches) != len(fixture.ExpectedBatchSizes) {
				t.Fatalf("%s: expected %d batches, got %d", fixture.Name, len(fixture.ExpectedBatchSizes), len(processor.batches))
			}
			for i, expectedSize := range fixture.ExpectedBatchSizes {
				if len(processor.batches[i]) != expectedSize {
					t.Fatalf("%s: expected batch %d size %d, got %d", fixture.Name, i, expectedSize, len(processor.batches[i]))
				}
			}
			if queue.Queue_len() != fixture.ExpectedRemainingQueueLen {
				t.Fatalf("%s: expected remaining queue len %d, got %d", fixture.Name, fixture.ExpectedRemainingQueueLen, queue.Queue_len())
			}
		})
	}
}

func TestParityFixtureFlushWakeups(t *testing.T) {
	fixtures := loadParityFixtures(t)
	for _, fixture := range fixtures.FlushWakeups {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			plan := BuildToolheadFlushHandlerPlan(fixture.Eventtime, fixture.EstimatedPrintTime, toFlushState(fixture.State), toFlushConfig(fixture.Config))
			if plan.ShouldFlushLookahead != fixture.Expected.ShouldFlushLookahead {
				t.Fatalf("%s: expected ShouldFlushLookahead=%v, got %v", fixture.Name, fixture.Expected.ShouldFlushLookahead, plan.ShouldFlushLookahead)
			}
			assertParityFloatSlice(t, fixture.Name, "AdvanceFlushTimes", plan.AdvanceFlushTimes, fixture.Expected.AdvanceFlushTimes)
			assertParityFloat(t, fixture.Name, "NextWakeTime", plan.NextWakeTime, fixture.Expected.NextWakeTime)
			if plan.ReturnNever != fixture.Expected.ReturnNever {
				t.Fatalf("%s: expected ReturnNever=%v, got %v", fixture.Name, fixture.Expected.ReturnNever, plan.ReturnNever)
			}
			if plan.KickFlushTimer != fixture.Expected.KickFlushTimer {
				t.Fatalf("%s: expected KickFlushTimer=%v, got %v", fixture.Name, fixture.Expected.KickFlushTimer, plan.KickFlushTimer)
			}
		})
	}
}

func TestParityFixtureTrapQTimelines(t *testing.T) {
	fixtures := loadParityFixtures(t)
	for _, fixture := range fixtures.TrapQTimelines {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			processor := &parityFixtureProcessor{}
			queue := NewMoveQueue(processor)
			calculator := &fakeMoveJunctionCalculator{result: fixture.JunctionResult}
			config := toMoveConfig(fixture.Config)
			for _, moveFixture := range fixture.Moves {
				queue.Add_move(NewMove(config, moveFixture.StartPos, moveFixture.EndPos, moveFixture.Speed), calculator)
			}
			queue.Flush(false)
			if len(processor.batches) != 1 {
				t.Fatalf("%s: expected one processed batch, got %d", fixture.Name, len(processor.batches))
			}
			moves := processor.batches[0]
			if len(moves) != len(fixture.Expected) {
				t.Fatalf("%s: expected %d timeline entries, got %d", fixture.Name, len(fixture.Expected), len(moves))
			}
			printTime := fixture.StartPrintTime
			for i, move := range moves {
				expected := fixture.Expected[i]
				moveTime := move.Accel_t + move.Cruise_t + move.Decel_t
				assertParityFloat(t, fixture.Name, "print_time", printTime, expected.PrintTime)
				assertParityFloat(t, fixture.Name, "start_v", move.Start_v, expected.StartV)
				assertParityFloat(t, fixture.Name, "cruise_v", move.Cruise_v, expected.CruiseV)
				assertParityFloat(t, fixture.Name, "accel", move.Accel, expected.Accel)
				assertParityFloat(t, fixture.Name, "move_time", moveTime, expected.MoveTime)
				printTime += moveTime
			}
		})
	}
}
