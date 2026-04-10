package motion

type MoveBatchSink interface {
	QueueKinematicMove(printTime float64, move *Move)
	QueueExtruderMove(printTime float64, move *Move)
}

func QueueMoveBatch(startPrintTime float64, moves []*Move, sink MoveBatchSink) float64 {
	nextMoveTime := startPrintTime
	for _, move := range moves {
		if move.Is_kinematic_move {
			sink.QueueKinematicMove(nextMoveTime, move)
		}
		if move.Axes_d[3] != 0.0 {
			sink.QueueExtruderMove(nextMoveTime, move)
		}
		nextMoveTime += move.Accel_t + move.Cruise_t + move.Decel_t
		for _, cb := range move.Timing_callbacks {
			cb(nextMoveTime)
		}
	}
	return nextMoveTime
}
