package mcu

type StepperPositionQuerySender interface {
	Send(data interface{}, minclock, reqclock int64) interface{}
}

func ApplyStepperHomingReset(queue StepcompressQueue, resetCmdTag int, oid int) error {
	state := StepperPositionState{}
	return state.NoteHomingEnd(queue, resetCmdTag, oid)
}

func ExecuteStepperPositionQuery(query StepperPositionQuerySender, oid int, commandedPosition float64, state *StepperPositionState, queue StepcompressQueue, receiveTimeFromResponse func(interface{}) float64, estimatedPrintTime func(float64) float64, printTimeToClock func(float64) int64) (int, error) {
	params := query.Send([]int64{int64(oid)}, 0, 0).(map[string]interface{})
	receiveTime := receiveTimeFromResponse(params["#receive_time"])
	return state.SyncFromQueryResponse(params["pos"].(int64), receiveTime, commandedPosition, estimatedPrintTime, printTimeToClock, queue)
}