package gcode

import (
	"goklipper/common/utils/cast"
	"math"
)

const (
	ArcPlaneXY = iota
	ArcPlaneXZ
	ArcPlaneYZ
)

const (
	XAxis = iota
	YAxis
	ZAxis
	EAxis
)

type ArcCommand interface {
	GetFloat(name string, defaultValue float64) float64
	GetFloatP(name string) *float64
}

type ArcState struct {
	GCodePosition        []float64
	AbsoluteCoordinates bool
	AbsoluteExtrude     bool
}

type LinearMoveHandler func(params map[string]string) error

type ArcSupport struct {
	mmPerArcSegment float64
	plane           int
}

func NewArcSupport(mmPerArcSegment float64) *ArcSupport {
	return &ArcSupport{
		mmPerArcSegment: mmPerArcSegment,
		plane:           ArcPlaneXY,
	}
}

func (self *ArcSupport) SetPlaneXY() {
	self.plane = ArcPlaneXY
}

func (self *ArcSupport) SetPlaneXZ() {
	self.plane = ArcPlaneXZ
}

func (self *ArcSupport) SetPlaneYZ() {
	self.plane = ArcPlaneYZ
}

func (self *ArcSupport) Plane() int {
	return self.plane
}

func (self *ArcSupport) CmdG2(gcmd ArcCommand, state ArcState, move LinearMoveHandler) error {
	return self.cmdInner(gcmd, true, state, move)
}

func (self *ArcSupport) CmdG3(gcmd ArcCommand, state ArcState, move LinearMoveHandler) error {
	return self.cmdInner(gcmd, false, state, move)
}

func (self *ArcSupport) cmdInner(gcmd ArcCommand, clockwise bool, state ArcState, move LinearMoveHandler) error {
	currentPos := state.GCodePosition
	if !state.AbsoluteCoordinates {
		panic("G2/G3 does not support relative move mode")
	}

	x := gcmd.GetFloat("X", currentPos[0])
	y := gcmd.GetFloat("Y", currentPos[1])
	z := gcmd.GetFloat("Z", currentPos[2])
	targetPos := []float64{x, y, z}

	if gcmd.GetFloatP("R") != nil {
		panic("G2/G3 does not support R moves")
	}

	asPlanar := make([]float64, 2)
	for i, a := range []string{"I", "J"} {
		asPlanar[i] = gcmd.GetFloat(a, 0.)
	}

	axes := []int{XAxis, YAxis, ZAxis}
	if self.plane == ArcPlaneXZ {
		for i, a := range []string{"I", "K"} {
			asPlanar[i] = gcmd.GetFloat(a, 0.)
		}
		axes = []int{XAxis, ZAxis, YAxis}
	} else if self.plane == ArcPlaneYZ {
		for i, a := range []string{"J", "K"} {
			asPlanar[i] = gcmd.GetFloat(a, 0.)
		}
		axes = []int{YAxis, ZAxis, XAxis}
	}

	if !(asPlanar[0] != 0 || asPlanar[1] != 0) {
		panic("G2/G3 requires IJ, IK or JK parameters")
	}

	return self.PlanArc(currentPos, targetPos, asPlanar, clockwise, gcmd,
		state.AbsoluteExtrude, axes[0], axes[1], axes[2], move)
}

func (self *ArcSupport) PlanArc(currentPos, targetPos, offset []float64,
	clockwise bool, gcmd ArcCommand, absoluteExtrude bool,
	alphaAxis, betaAxis, helicalAxis int, move LinearMoveHandler) error {
	rP := -offset[0]
	rQ := -offset[1]

	centerP := currentPos[alphaAxis] - rP
	centerQ := currentPos[betaAxis] - rQ

	rtAlpha := targetPos[alphaAxis] - centerP
	rtBeta := targetPos[betaAxis] - centerQ

	angularTravel := math.Atan2(rP*rtBeta-rQ*rtAlpha, rP*rtAlpha+rQ*rtBeta)
	if angularTravel < 0 {
		angularTravel += 2 * math.Pi
	}
	if clockwise {
		angularTravel -= 2 * math.Pi
	}

	if angularTravel == 0 &&
		currentPos[alphaAxis] == targetPos[alphaAxis] &&
		currentPos[betaAxis] == targetPos[betaAxis] {
		angularTravel = 2 * math.Pi
	}

	linearTravel := targetPos[helicalAxis] - currentPos[helicalAxis]
	radius := math.Hypot(rP, rQ)
	flatMM := radius * angularTravel
	var mmOfTravel float64
	if linearTravel != 0 {
		mmOfTravel = math.Hypot(flatMM, linearTravel)
	} else {
		mmOfTravel = math.Abs(flatMM)
	}
	segments := math.Max(1, math.Floor(mmOfTravel/self.mmPerArcSegment))

	thetaPerSegment := angularTravel / segments
	linearPerSegment := linearTravel / segments

	ePerMove, eBase := 0.0, 0.0
	asE := gcmd.GetFloatP("E")
	asF := gcmd.GetFloatP("F")

	if asE != nil {
		if absoluteExtrude {
			eBase = currentPos[EAxis]
		}
		ePerMove = (*asE - eBase) / float64(segments)
	}

	for i := 1; i <= int(segments); i++ {
		distHelical := float64(i) * linearPerSegment
		cTheta := float64(i) * thetaPerSegment
		cosTi := math.Cos(cTheta)
		sinTi := math.Sin(cTheta)
		rP = -offset[0]*cosTi + offset[1]*sinTi
		rQ = -offset[0]*sinTi - offset[1]*cosTi

		coord := []float64{
			centerP + rP,
			centerQ + rQ,
			currentPos[helicalAxis] + distHelical,
		}

		if i == int(segments) {
			coord = targetPos
		}

		g1Params := map[string]string{
			"X": cast.ToString(coord[0]),
			"Y": cast.ToString(coord[1]),
			"Z": cast.ToString(coord[2]),
		}

		if ePerMove != 0 {
			g1Params["E"] = cast.ToString(eBase + ePerMove)
			if absoluteExtrude {
				eBase += ePerMove
			}
		}

		if asF != nil {
			g1Params["F"] = cast.ToString(*asF)
		}

		if err := move(g1Params); err != nil {
			return err
		}
	}

	return nil
}