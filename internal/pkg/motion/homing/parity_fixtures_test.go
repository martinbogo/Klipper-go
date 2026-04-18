package homing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type parityHomingStepperFixture struct {
	Name              string  `json:"name"`
	CommandedPosition float64 `json:"commanded_position"`
	TriggeredPosition int     `json:"triggered_position"`
}

type parityHomingOutcomeExpectedFixture struct {
	Error            string  `json:"error"`
	SetPositionCalls int     `json:"set_position_calls"`
	FinalX           float64 `json:"final_x"`
	TriggerX         float64 `json:"trigger_x"`
}

type parityHomeRailsOutcomeFixture struct {
	Name                string                             `json:"name"`
	ForcePos            []interface{}                      `json:"forcepos"`
	MovePos             []interface{}                      `json:"movepos"`
	TriggerPos          []float64                          `json:"trigger_pos"`
	TriggerTime         float64                            `json:"trigger_time"`
	Stepper             parityHomingStepperFixture         `json:"stepper"`
	AfterHomeAdjustment float64                            `json:"after_home_adjustment"`
	AfterHomeError      string                             `json:"after_home_error"`
	Expected            parityHomingOutcomeExpectedFixture `json:"expected"`
}

type parityHomingFixtures struct {
	HomeRailsOutcomes []parityHomeRailsOutcomeFixture `json:"home_rails_outcomes"`
}

func loadHomingParityFixtures(t *testing.T) parityHomingFixtures {
	t.Helper()
	path := filepath.Join("testdata", "parity_fixtures.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read homing parity fixtures: %v", err)
	}
	var fixtures parityHomingFixtures
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("parse homing parity fixtures: %v", err)
	}
	return fixtures
}

func TestParityFixtureHomeRailsOutcomes(t *testing.T) {
	fixtures := loadHomingParityFixtures(t)
	for _, fixture := range fixtures.HomeRailsOutcomes {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			stepper := &fakeStepper{
				name:              fixture.Stepper.Name,
				stepDist:          1,
				commandedPosition: fixture.Stepper.CommandedPosition,
			}
			toolhead := &fakeDripToolhead{
				position:     []float64{0, 0, 0, 0},
				kinematics:   &fakeKinematics{steppers: []Stepper{stepper}},
				lastMoveTime: 2.0,
			}
			state := NewState(toolhead)
			rail := &fakeRail{
				endstops: []NamedEndstop{{Endstop: &fakeEndstop{}, Name: "x_endstop"}},
				info: &RailHomingInfo{
					Speed:             50,
					PositionEndstop:   10,
					RetractSpeed:      20,
					RetractDist:       0,
					PositiveDir:       false,
					SecondHomingSpeed: 25,
				},
			}
			move := &fakeMoveExecutor{
				triggerPos:  append([]float64{}, fixture.TriggerPos...),
				triggerTime: fixture.TriggerTime,
				stepperPosition: []*StepperPosition{{
					Stepper:     stepper,
					EndstopName: "x_endstop",
					StepperName: fixture.Stepper.Name,
					TrigPos:     fixture.Stepper.TriggeredPosition,
				}},
			}

			err := state.HomeRailsWithPositions(
				[]Rail{rail},
				append([]interface{}{}, fixture.ForcePos...),
				append([]interface{}{}, fixture.MovePos...),
				func(endstops []NamedEndstop) MoveExecutor {
					if len(endstops) != 1 || endstops[0].Name != "x_endstop" {
						t.Fatalf("unexpected endstops %#v", endstops)
					}
					return move
				},
				func() error {
					state.SetStepperAdjustment(fixture.Stepper.Name, fixture.AfterHomeAdjustment)
					if fixture.AfterHomeError != "" {
						return errors.New(fixture.AfterHomeError)
					}
					return nil
				},
			)

			if fixture.Expected.Error == "" {
				if err != nil {
					t.Fatalf("expected no home rails error, got %v", err)
				}
			} else {
				if err == nil || err.Error() != fixture.Expected.Error {
					t.Fatalf("expected home rails error %q, got %v", fixture.Expected.Error, err)
				}
			}
			if len(toolhead.setPositionCalls) != fixture.Expected.SetPositionCalls {
				t.Fatalf("expected %d set-position calls, got %#v", fixture.Expected.SetPositionCalls, toolhead.setPositionCalls)
			}
			if len(toolhead.setPositionCalls) == 0 {
				t.Fatalf("expected at least one set-position call, got %#v", toolhead.setPositionCalls)
			}
			last := toolhead.setPositionCalls[len(toolhead.setPositionCalls)-1]
			if last[0] != fixture.Expected.FinalX {
				t.Fatalf("expected final X %v, got %#v", fixture.Expected.FinalX, last)
			}
			if got := state.GetTriggerPosition(fixture.Stepper.Name); got != fixture.Expected.TriggerX {
				t.Fatalf("expected trigger position %v, got %v", fixture.Expected.TriggerX, got)
			}
		})
	}
}
