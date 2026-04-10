package probe

type FinalizeCallback func([]float64, [][]float64) string

type ProbePointsHelper struct {
	finalizeCallback FinalizeCallback
	probePoints      [][]float64
	name             string
	horizontalMoveZ  float64
	speed            float64
	useOffsets       bool
	liftSpeed        float64
	probeOffsets     []float64
	results          [][]float64
}

func NewProbePointsHelper(name string, finalizeCallback FinalizeCallback, defaultPoints [][]float64, horizontalMoveZ, speed float64) *ProbePointsHelper {
	points := make([][]float64, len(defaultPoints))
	for i, point := range defaultPoints {
		copied := make([]float64, len(point))
		copy(copied, point)
		points[i] = copied
	}
	return &ProbePointsHelper{
		finalizeCallback: finalizeCallback,
		probePoints:      points,
		name:             name,
		horizontalMoveZ:  horizontalMoveZ,
		speed:            speed,
		liftSpeed:        speed,
		probeOffsets:     []float64{0., 0., 0.},
		results:          [][]float64{},
	}
}

func (self *ProbePointsHelper) Name() string {
	return self.name
}

func (self *ProbePointsHelper) HorizontalMoveZ() float64 {
	return self.horizontalMoveZ
}

func (self *ProbePointsHelper) Speed() float64 {
	return self.speed
}

func (self *ProbePointsHelper) LiftSpeed() float64 {
	return self.liftSpeed
}

func (self *ProbePointsHelper) ProbeOffsets() []float64 {
	copied := make([]float64, len(self.probeOffsets))
	copy(copied, self.probeOffsets)
	return copied
}

func (self *ProbePointsHelper) ResultCount() int {
	return len(self.results)
}

func (self *ProbePointsHelper) MinimumPoints(n int) bool {
	return len(self.probePoints) >= n
}

func (self *ProbePointsHelper) UpdateProbePoints(points [][]float64) {
	updated := make([][]float64, len(points))
	for i, point := range points {
		copied := make([]float64, len(point))
		copy(copied, point)
		updated[i] = copied
	}
	self.probePoints = updated
}

func (self *ProbePointsHelper) UseXYOffsets(useOffsets bool) {
	self.useOffsets = useOffsets
}

func (self *ProbePointsHelper) BeginManualSession() {
	self.results = [][]float64{}
	self.liftSpeed = self.speed
	self.probeOffsets = []float64{0., 0., 0.}
}

func (self *ProbePointsHelper) BeginAutomaticSession(liftSpeed float64, probeOffsets []float64) {
	self.results = [][]float64{}
	self.liftSpeed = liftSpeed
	self.probeOffsets = make([]float64, len(probeOffsets))
	copy(self.probeOffsets, probeOffsets)
}

func (self *ProbePointsHelper) AppendResult(pos []float64) {
	copied := make([]float64, len(pos))
	copy(copied, pos)
	self.results = append(self.results, copied)
}

func (self *ProbePointsHelper) NextProbePoint() (done bool, retry bool, target []float64) {
	if len(self.results) >= len(self.probePoints) {
		res := self.finalizeCallback(self.ProbeOffsets(), self.results)
		if res != "retry" {
			return true, false, nil
		}
		self.results = [][]float64{}
		return false, true, self.currentTarget()
	}
	return false, false, self.currentTarget()
}

func (self *ProbePointsHelper) currentTarget() []float64 {
	nextpos := make([]float64, len(self.probePoints[len(self.results)]))
	copy(nextpos, self.probePoints[len(self.results)])
	if self.useOffsets && len(nextpos) >= 2 {
		nextpos[0] -= self.probeOffsets[0]
		nextpos[1] -= self.probeOffsets[1]
	}
	return nextpos
}
