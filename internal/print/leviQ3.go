package print

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	default_platform_x_size            = 250.0
	default_platform_y_size            = 250.0
	default_platform_z_size            = 260.0
	default_bed_temp                   = 60.0
	default_first_layer_bed_temp       = 60.0
	default_leveling_swiping_temp      = 55.0
	default_leveling_swiping_temp_diff = 10.0
	default_homing_speed               = 80.0
	default_second_homing_speed        = 25.0
	default_homing_retract_dist        = 5.0
	default_homing_retract_speed       = 20.0
	default_homing_retry_count         = 2
	default_scratch_sensitivity        = 0.04
	default_auto_zoffset_max_diff      = 0.0401
	default_auto_zoffset_noise_diff    = 0.0401
	default_auto_zoffset_retry_count   = 2
	default_auto_zoffset_step_limit    = 5.0
	default_sample_retract_dist        = 2.0
	default_recover_velocity           = 5.0
	default_expansion_factor           = 0.0
	auto_zoffset_reference_temp        = 140.0
	auto_zoffset_temperature_span      = 80.0
	default_preheat_recheck_margin     = 5.0
	default_wipe_travel_step           = 15.0
	default_hotbed_timeout             = 5 * time.Minute
)

func ComputeLeviQ3TemperatureCompensation(temperature float64, expansionFactor float64) float64 {
	if temperature <= 0 {
		return 0
	}
	return (temperature - auto_zoffset_reference_temp) * (expansionFactor / auto_zoffset_temperature_span)
}

type Axis string

const (
	AxisX Axis = "x"
	AxisY Axis = "y"
	AxisZ Axis = "z"
)

type XY struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type XYZ struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type XYCount struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type ProbePoint struct {
	Index     int     `json:"index"`
	Position  XY      `json:"position"`
	MeasuredZ float64 `json:"measured_z"`
	Valid     bool    `json:"valid"`
}

type BedMesh struct {
	mesh_min XY
	mesh_max XY
	count    XYCount
	matrix   [][]float64
	points   []ProbePoint
	source   string
}

type BedMeshSnapshot struct {
	MeshMin XY           `json:"mesh_min"`
	MeshMax XY           `json:"mesh_max"`
	Count   XYCount      `json:"count"`
	Matrix  [][]float64  `json:"matrix"`
	Points  []ProbePoint `json:"points,omitempty"`
	Source  string       `json:"source,omitempty"`
}

func cloneBedMeshMatrix(matrix [][]float64) [][]float64 {
	out := make([][]float64, len(matrix))
	for y := range matrix {
		out[y] = append([]float64(nil), matrix[y]...)
	}
	return out
}

func NewBedMesh(mesh_min, mesh_max XY, count XYCount, source string) *BedMesh {
	if count.X < 1 {
		count.X = 1
	}
	if count.Y < 1 {
		count.Y = 1
	}

	matrix := make([][]float64, count.Y)
	for y := range matrix {
		matrix[y] = make([]float64, count.X)
	}

	return &BedMesh{
		mesh_min: mesh_min,
		mesh_max: mesh_max,
		count:    count,
		matrix:   matrix,
		source:   source,
	}
}

func NewBedMeshFromSnapshot(snapshot *BedMeshSnapshot) *BedMesh {
	if snapshot == nil {
		return nil
	}
	count := snapshot.Count
	if count.X < 1 || count.Y < 1 {
		if len(snapshot.Matrix) > 0 && len(snapshot.Matrix[0]) > 0 {
			count = XYCount{X: len(snapshot.Matrix[0]), Y: len(snapshot.Matrix)}
		}
	}
	mesh := NewBedMesh(snapshot.MeshMin, snapshot.MeshMax, count, snapshot.Source)
	if len(snapshot.Matrix) > 0 && len(snapshot.Matrix[0]) > 0 {
		mesh.matrix = cloneBedMeshMatrix(snapshot.Matrix)
		mesh.count = XYCount{X: len(mesh.matrix[0]), Y: len(mesh.matrix)}
	}
	mesh.points = append([]ProbePoint(nil), snapshot.Points...)
	return mesh
}

func (m *BedMesh) Set(x, y int, z float64) {
	if m == nil || y < 0 || y >= len(m.matrix) || x < 0 || x >= len(m.matrix[y]) {
		return
	}
	m.matrix[y][x] = z
}

func (m *BedMesh) CloneMatrix() [][]float64 {
	if m == nil {
		return nil
	}
	return cloneBedMeshMatrix(m.matrix)
}

func (m *BedMesh) Snapshot() *BedMeshSnapshot {
	if m == nil {
		return nil
	}
	return &BedMeshSnapshot{
		MeshMin: m.mesh_min,
		MeshMax: m.mesh_max,
		Count:   m.count,
		Matrix:  m.CloneMatrix(),
		Points:  append([]ProbePoint(nil), m.points...),
		Source:  m.source,
	}
}

func (m *BedMesh) Average() float64 {
	if m == nil || len(m.points) == 0 {
		return 0
	}
	var sum float64
	var n int
	for _, point := range m.points {
		if !point.Valid {
			continue
		}
		sum += point.MeasuredZ
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func (m *BedMesh) Print_probed_matrix() string {
	if m == nil {
		return "<nil mesh>"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Mesh X,Y: %d,%d\n", m.count.X, m.count.Y)
	for y := range m.matrix {
		for x := range m.matrix[y] {
			fmt.Fprintf(&b, "%8.4f", m.matrix[y][x])
		}
		b.WriteByte('\n')
	}
	return b.String()
}

type TemperatureProfile struct {
	bed_temp                    float64
	first_layer_bed_temperature float64
	defaultLevelingSwipingTemp  float64
	levelingSwipingTempDiff     float64
	HotbedTemperatureTimeout    time.Duration
}

type HomingParams struct {
	homing_speed         float64
	second_homing_speed  float64
	homing_retract_dist  float64
	homing_retract_speed float64
	homing_positive_dir  bool
	homing_retry_count   int
}

func (h HomingParams) HomingSpeed() float64 {
	return h.homing_speed
}

func (h HomingParams) SecondHomingSpeed() float64 {
	return h.second_homing_speed
}

func (h HomingParams) HomingRetractDist() float64 {
	return h.homing_retract_dist
}

func (h HomingParams) HomingRetractSpeed() float64 {
	return h.homing_retract_speed
}

func (h HomingParams) HomingPositiveDir() bool {
	return h.homing_positive_dir
}

func (h HomingParams) HomingRetryCount() int {
	return h.homing_retry_count
}

type MeshConfig struct {
	mesh_min       XY
	mesh_max       XY
	probe_count    XYCount
	mesh_offsets   XYZ
	fade_offset    float64
	use_xy_offsets bool
}

type LeviQ3PersistentState struct {
	SavedZOffset           float64
	LastHotbedTemp         float64
	LastCompensationTemp   float64
	LastTempCompensation   float64
	LastAutoZOffsetRaw     float64
	LastAutoZOffset        float64
	LastAutoZOffsetSamples []float64
	LastAppliedOffsets     XYZ
	LastCancelReason       string
	LastRun                time.Time
	SavedMesh              *BedMesh
}

type StatusSink interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

type NullStatus struct{}

func (NullStatus) Infof(string, ...any)  {}
func (NullStatus) Errorf(string, ...any) {}

type FuncStatusSink struct {
	InfofFunc  func(format string, args ...any)
	ErrorfFunc func(format string, args ...any)
}

func (s FuncStatusSink) Infof(format string, args ...any) {
	if s.InfofFunc != nil {
		s.InfofFunc(format, args...)
	}
}

func (s FuncStatusSink) Errorf(format string, args ...any) {
	if s.ErrorfFunc != nil {
		s.ErrorfFunc(format, args...)
	}
}

type ConfigSource interface {
	Float64(key string, fallback float64) float64
	Int(key string, fallback int) int
	Bool(key string, fallback bool) bool
	Float64Slice(key string, fallback []float64) []float64
	String(key string, fallback string) string
}

type FuncConfigSource struct {
	Float64Func      func(key string, fallback float64) float64
	IntFunc          func(key string, fallback int) int
	BoolFunc         func(key string, fallback bool) bool
	Float64SliceFunc func(key string, fallback []float64) []float64
	StringFunc       func(key string, fallback string) string
}

func (s FuncConfigSource) Float64(key string, fallback float64) float64 {
	if s.Float64Func != nil {
		return s.Float64Func(key, fallback)
	}
	return fallback
}

func (s FuncConfigSource) Int(key string, fallback int) int {
	if s.IntFunc != nil {
		return s.IntFunc(key, fallback)
	}
	return fallback
}

func (s FuncConfigSource) Bool(key string, fallback bool) bool {
	if s.BoolFunc != nil {
		return s.BoolFunc(key, fallback)
	}
	return fallback
}

func (s FuncConfigSource) Float64Slice(key string, fallback []float64) []float64 {
	if s.Float64SliceFunc != nil {
		return s.Float64SliceFunc(key, fallback)
	}
	return append([]float64(nil), fallback...)
}

func (s FuncConfigSource) String(key string, fallback string) string {
	if s.StringFunc != nil {
		return s.StringFunc(key, fallback)
	}
	return fallback
}

type MotionController interface {
	Is_printer_ready(ctx context.Context) bool
	Clear_homing_state(ctx context.Context) error
	Home_axis(ctx context.Context, axis Axis, params HomingParams) error
	Home_rails(ctx context.Context, axes []Axis, params HomingParams) error
	Homing_move(ctx context.Context, axis Axis, target float64, speed float64) error
	Set_bed_temperature(ctx context.Context, target float64) error
	Wait_for_temperature(ctx context.Context, target float64, timeout time.Duration) error
	GetHotbedTemp(ctx context.Context) (float64, error)
	Lower_probe(ctx context.Context) error
	Raise_probe(ctx context.Context) error
	Run_probe(ctx context.Context, position XY) (float64, error)
	Wipe_nozzle(ctx context.Context, position XYZ) error
	Set_gcode_offset(ctx context.Context, z float64) error
	Current_z_offset(ctx context.Context) float64
	Save_mesh(ctx context.Context, mesh *BedMesh) error
	Sleep(ctx context.Context, d time.Duration) error
}

type AutoZOffsetTemperatureSource interface {
	GetAutoZOffsetTemperature(ctx context.Context) (float64, error)
}

type AutoZOffsetMeasurementSource interface {
	MeasureAutoZOffset(ctx context.Context, helper *LeviQ3Helper) (float64, error)
}

type ProbeOffsetSyncTarget interface {
	Z_offset_apply_probe(ctx context.Context, z float64) error
}

type AbsoluteProbeOffsetSyncTarget interface {
	Z_offset_apply_probe_absolute(ctx context.Context, z float64) error
}

type MeshOffsetSyncTarget interface {
	Set_mesh_offsets(ctx context.Context, offsets XYZ) error
}

type RailsOffsetSyncTarget interface {
	Set_rails_z_offset(ctx context.Context, z float64) error
}

type LeviQ3CancelTarget interface {
	CancelLeviQ3(ctx context.Context, reason string) error
}

type LeviQ3WipeSequenceProvider interface {
	LeviQ3WipeSequence(ctx context.Context, helper *LeviQ3Helper) ([]XYZ, error)
}

type MapConfigSource map[string]any

func (m MapConfigSource) Float64(key string, fallback float64) float64 {
	value, ok := m[key]
	if !ok {
		return fallback
	}
	if out, ok := toFloat64(value); ok {
		return out
	}
	return fallback
}

func (m MapConfigSource) Int(key string, fallback int) int {
	value, ok := m[key]
	if !ok {
		return fallback
	}
	if out, ok := toFloat64(value); ok {
		return int(out)
	}
	return fallback
}

func (m MapConfigSource) Bool(key string, fallback bool) bool {
	value, ok := m[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		return s == "1" || s == "true" || s == "on" || s == "enable" || s == "enabled"
	default:
		return fallback
	}
}

func (m MapConfigSource) Float64Slice(key string, fallback []float64) []float64 {
	value, ok := m[key]
	if !ok {
		return append([]float64(nil), fallback...)
	}
	if out, ok := toFloat64Slice(value); ok {
		return out
	}
	return append([]float64(nil), fallback...)
}

func (m MapConfigSource) String(key string, fallback string) string {
	value, ok := m[key]
	if !ok {
		return fallback
	}
	if out, ok := value.(string); ok {
		return out
	}
	return fallback
}

type LeviQ3Helper struct {
	config_source ConfigSource
	motion        MotionController
	status        StatusSink

	platform_x_size       float64
	platform_y_size       float64
	platform_z_size       float64
	platform_offset       XYZ
	rough_platform_offset XYZ

	probe_offsets            XYZ
	mesh_offsets             XYZ
	zoffset_base             float64
	current_zoffset          float64
	temperature_compensation float64
	expansion_factor         float64
	max_diff                 float64
	noise_diff               float64
	sample_retract_dist      float64
	recover_velocity         float64
	preheat_recheck_margin   float64
	auto_zoffset_retry_count int
	last_valid_auto_zoffset  float64

	mesh_config   MeshConfig
	probe_points  []XY
	probed_matrix *BedMesh

	homing_pos_xy   XY
	wiping_position XYZ
	wiping_sequence []XYZ
	homing_params   HomingParams
	temperature     TemperatureProfile
	kinematics      string

	scratch_sensitivity float64
	preheat_leveling    bool
	auto_zoffset_on_off bool
	use_xy_offsets      bool
	is_homing           bool
	ready               bool
	is_scratch_notice   bool
	debug_g9113         bool
	cancel_requested    bool
	cancel_reason       string

	last_probe_samples [3]float64
	last_probe_errors  [3]float64
	saved_config       LeviQ3PersistentState
}

func Load_config_LeviQ3(config_source ConfigSource, motion MotionController, status StatusSink) (*LeviQ3Helper, error) {
	return NewLeviQ3Helper(config_source, motion, status)
}

func NewLeviQ3Helper(config_source ConfigSource, motion MotionController, status StatusSink) (*LeviQ3Helper, error) {
	if config_source == nil {
		config_source = MapConfigSource{}
	}
	if status == nil {
		status = NullStatus{}
	}

	helper := &LeviQ3Helper{
		config_source:            config_source,
		motion:                   motion,
		status:                   status,
		platform_x_size:          default_platform_x_size,
		platform_y_size:          default_platform_y_size,
		platform_z_size:          default_platform_z_size,
		expansion_factor:         default_expansion_factor,
		max_diff:                 default_auto_zoffset_max_diff,
		noise_diff:               default_auto_zoffset_noise_diff,
		sample_retract_dist:      default_sample_retract_dist,
		recover_velocity:         default_recover_velocity,
		preheat_recheck_margin:   default_preheat_recheck_margin,
		auto_zoffset_retry_count: default_auto_zoffset_retry_count,
		scratch_sensitivity:      default_scratch_sensitivity,
		preheat_leveling:         true,
		auto_zoffset_on_off:      true,
		use_xy_offsets:           true,
		kinematics:               "cartesian",
		ready:                    true,
	}

	if err := helper.Build_config(); err != nil {
		return nil, err
	}
	return helper, nil
}

func (h *LeviQ3Helper) Build_config() error {
	if h == nil {
		return errors.New("nil LeviQ3Helper")
	}

	h.platform_x_size = h.config_source.Float64("platform_x_size", h.platform_x_size)
	h.platform_y_size = h.config_source.Float64("platform_y_size", h.platform_y_size)
	h.platform_z_size = h.config_source.Float64("platform_z_size", h.platform_z_size)
	h.platform_offset = xyz_from_slice(h.config_source.Float64Slice("platform_offset", []float64{0, 0, 0}))
	h.rough_platform_offset = xyz_from_slice(h.config_source.Float64Slice("rough_platform_offset", []float64{0, 0, 0}))
	h.probe_offsets = xyz_from_slice(h.config_source.Float64Slice("probe_offsets", []float64{0, 0, 0}))
	h.mesh_offsets = xyz_from_slice(h.config_source.Float64Slice("Mesh_offsets", []float64{0, 0, 0}))

	h.zoffset_base = h.config_source.Float64("zoffset_base", 0)
	h.current_zoffset = h.config_source.Float64("zoffset", h.zoffset_base)
	h.saved_config.SavedZOffset = h.current_zoffset
	h.expansion_factor = h.config_source.Float64("expansion_factor", h.config_source.Float64("offsetFreq", h.expansion_factor))
	h.max_diff = h.config_source.Float64("max_diff", h.max_diff)
	h.noise_diff = h.config_source.Float64("noise_diff", h.noise_diff)
	h.sample_retract_dist = h.config_source.Float64("Sample_retract_dist", h.config_source.Float64("sample_retract_dist", h.sample_retract_dist))
	h.recover_velocity = h.config_source.Float64("recover_velocity", h.recover_velocity)
	h.preheat_recheck_margin = h.config_source.Float64("preheat_recheck_margin", h.preheat_recheck_margin)
	h.auto_zoffset_retry_count = h.config_source.Int("auto_zoffset_retry_count", h.auto_zoffset_retry_count)

	h.mesh_config.mesh_min = xy_from_slice(h.config_source.Float64Slice("mesh_min", []float64{15, 15}))
	h.mesh_config.mesh_max = xy_from_slice(h.config_source.Float64Slice("mesh_max", []float64{h.platform_x_size - 15, h.platform_y_size - 15}))
	h.mesh_config.fade_offset = h.config_source.Float64("Fade_offset", 0)
	h.mesh_config.use_xy_offsets = h.config_source.Bool("Use_xy_offsets", true)
	h.mesh_config.mesh_offsets = h.mesh_offsets
	h.mesh_config.probe_count = xy_count_from_slice(h.config_source.Float64Slice("probe_count", []float64{4, 4}), XYCount{X: 4, Y: 4})

	h.homing_pos_xy = xy_from_slice(h.config_source.Float64Slice("homing_pos_xy", []float64{15, 15}))
	h.wiping_position = xyz_from_slice(h.config_source.Float64Slice("wiping_position", []float64{h.platform_x_size - 10, h.platform_y_size - 10, 5}))
	h.wiping_sequence = xyz_triples_from_slice(h.config_source.Float64Slice("wiping_sequence", nil))
	if len(h.wiping_sequence) == 0 {
		h.wiping_sequence = h.build_wiping_sequence()
	}

	h.homing_params = HomingParams{
		homing_speed:         h.config_source.Float64("homing_speed", default_homing_speed),
		second_homing_speed:  h.config_source.Float64("second_homing_speed", default_second_homing_speed),
		homing_retract_dist:  h.config_source.Float64("homing_retract_dist", default_homing_retract_dist),
		homing_retract_speed: h.config_source.Float64("homing_retract_speed", default_homing_retract_speed),
		homing_positive_dir:  h.config_source.Bool("homing_positive_dir", false),
		homing_retry_count:   h.config_source.Int("homing_retry_count", default_homing_retry_count),
	}

	h.temperature = TemperatureProfile{
		bed_temp:                    h.config_source.Float64("bed_temp", default_bed_temp),
		first_layer_bed_temperature: h.config_source.Float64("first_layer_bed_temperature", default_first_layer_bed_temp),
		defaultLevelingSwipingTemp:  h.config_source.Float64("defaultLevelingSwipingTemp", default_leveling_swiping_temp),
		levelingSwipingTempDiff:     h.config_source.Float64("levelingSwipingTempDiff", default_leveling_swiping_temp_diff),
		HotbedTemperatureTimeout:    default_hotbed_timeout,
	}

	h.kinematics = strings.ToLower(strings.TrimSpace(h.config_source.String("kinematics", h.kinematics)))
	h.preheat_leveling = h.config_source.Bool("preheat_leveling", true)
	h.scratch_sensitivity = h.config_source.Float64("scratch_sensitivity", default_scratch_sensitivity)
	h.auto_zoffset_on_off = h.config_source.Bool("auto_zoffset_on_off", true)
	h.use_xy_offsets = h.mesh_config.use_xy_offsets

	probe_points := xy_pairs_from_slice(h.config_source.Float64Slice("probe_points", nil))
	if len(probe_points) == 0 {
		probe_points = h.build_probe_points()
	}
	h.Update_probe_points(probe_points)
	h.saved_config.LastAppliedOffsets = h.Get_offsets()

	h.status.Infof("Build_config: LeviQ3 probe grid=%dx%d, points=%d, kinematics=%s", h.mesh_config.probe_count.X, h.mesh_config.probe_count.Y, len(h.probe_points), h.kinematics)
	return nil
}

func (h *LeviQ3Helper) PersistentState() LeviQ3PersistentState {
	if h == nil {
		return LeviQ3PersistentState{}
	}
	state := h.saved_config
	state.LastAutoZOffsetSamples = append([]float64(nil), h.saved_config.LastAutoZOffsetSamples...)
	return state
}

func (h *LeviQ3Helper) RestorePersistentState(state LeviQ3PersistentState) {
	if h == nil {
		return
	}
	h.saved_config = state
	h.saved_config.LastAutoZOffsetSamples = append([]float64(nil), state.LastAutoZOffsetSamples...)
	if state.SavedZOffset != 0 || h.current_zoffset == 0 {
		h.current_zoffset = state.SavedZOffset
	}
	h.last_valid_auto_zoffset = state.LastAutoZOffset
	if state.LastAppliedOffsets != (XYZ{}) {
		h.saved_config.LastAppliedOffsets = state.LastAppliedOffsets
	}
	if state.SavedMesh != nil {
		h.probed_matrix = state.SavedMesh
	}
}

func (h *LeviQ3Helper) RestorePersistentStateFromJSON(payload []byte) error {
	if h == nil {
		return errors.New("nil LeviQ3Helper")
	}
	if len(payload) == 0 {
		return nil
	}
	var state LeviQ3PersistedStateRecord
	if err := json.Unmarshal(payload, &state); err != nil {
		return err
	}
	h.RestorePersistentState(state.PersistentState())
	return nil
}

func (h *LeviQ3Helper) RestorePersistentStateValue(raw any) error {
	if h == nil {
		return errors.New("nil LeviQ3Helper")
	}
	if raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case string:
		payload := []byte(strings.TrimSpace(typed))
		if len(payload) == 0 {
			return nil
		}
		return h.RestorePersistentStateFromJSON(payload)
	case []byte:
		if len(typed) == 0 {
			return nil
		}
		return h.RestorePersistentStateFromJSON(typed)
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return h.RestorePersistentStateFromJSON(payload)
}

func (h *LeviQ3Helper) RestorePersistentStateVariable(variables map[string]interface{}, variable string) error {
	if h == nil {
		return errors.New("nil LeviQ3Helper")
	}
	variable = strings.TrimSpace(variable)
	if variable == "" {
		return errors.New("empty save-variable name")
	}
	if len(variables) == 0 {
		return nil
	}
	return h.RestorePersistentStateValue(variables[variable])
}

func (h *LeviQ3Helper) PersistentStateJSON() ([]byte, error) {
	if h == nil {
		return nil, errors.New("nil LeviQ3Helper")
	}
	persisted := NewLeviQ3PersistedStateRecord(h.CurrentZOffset(), h.PersistentState())
	return json.Marshal(persisted)
}

func (h *LeviQ3Helper) PersistentStateSaveVariableValue() (string, error) {
	payload, err := h.PersistentStateJSON()
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (h *LeviQ3Helper) PersistentStateSaveVariableCommand(variable string) (string, error) {
	if h == nil {
		return "", errors.New("nil LeviQ3Helper")
	}
	variable = strings.TrimSpace(variable)
	if variable == "" {
		return "", errors.New("empty save-variable name")
	}
	value, err := h.PersistentStateSaveVariableValue()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SAVE_VARIABLE VARIABLE=%s VALUE=%s", variable, value), nil
}

func (h *LeviQ3Helper) MeshBuildParams() map[string]interface{} {
	if h == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"min_x":      h.mesh_config.mesh_min.X,
		"max_x":      h.mesh_config.mesh_max.X,
		"min_y":      h.mesh_config.mesh_min.Y,
		"max_y":      h.mesh_config.mesh_max.Y,
		"x_count":    h.mesh_config.probe_count.X,
		"y_count":    h.mesh_config.probe_count.Y,
		"mesh_x_pps": 2,
		"mesh_y_pps": 2,
		"algo":       "lagrange",
		"tension":    0.2,
	}
}

func (h *LeviQ3Helper) CurrentZOffset() float64 {
	if h == nil {
		return 0
	}
	return h.current_zoffset
}

func (h *LeviQ3Helper) SetHomingRetryCount(count int) {
	if h == nil {
		return
	}
	if count < 0 {
		count = 0
	}
	h.homing_params.homing_retry_count = count
}

func (h *LeviQ3Helper) PlatformOffsets(includeRough ...bool) XYZ {
	if h == nil {
		return XYZ{}
	}
	offsets := h.platform_offset
	if len(includeRough) > 0 && includeRough[0] {
		offsets = add_xyz(offsets, h.rough_platform_offset)
	}
	return offsets
}

func (h *LeviQ3Helper) handle_ready(ctx context.Context) error {
	if h.motion == nil {
		h.ready = true
		return nil
	}
	for {
		if err := h.check_cancel(ctx, "handle_ready"); err != nil {
			return err
		}
		if h.motion.Is_printer_ready(ctx) {
			h.ready = true
			return nil
		}
		select {
		case <-ctx.Done():
			h.CancelEvent(ctx.Err().Error())
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (h *LeviQ3Helper) clear_cancel() {
	h.cancel_requested = false
	h.cancel_reason = ""
	h.saved_config.LastCancelReason = ""
	h.ready = true
}

func (h *LeviQ3Helper) ResetCancelState() {
	if h == nil {
		return
	}
	h.clear_cancel()
}

func (h *LeviQ3Helper) CancelEvent(reason ...string) {
	if h == nil {
		return
	}
	message := "leviq3 cancelled"
	if len(reason) > 0 {
		if trimmed := strings.TrimSpace(reason[0]); trimmed != "" {
			message = trimmed
		}
	}
	h.cancel_requested = true
	h.cancel_reason = message
	h.saved_config.LastCancelReason = message
	h.is_homing = false
	h.ready = false
	h.status.Errorf("CancelEvent: %s", message)
	if target, ok := h.motion.(LeviQ3CancelTarget); ok {
		_ = target.CancelLeviQ3(context.Background(), message)
	}
}

func (h *LeviQ3Helper) check_cancel(ctx context.Context, stage string) error {
	if err := ctx.Err(); err != nil {
		h.CancelEvent(err.Error())
		return err
	}
	if h.cancel_requested {
		if stage == "" {
			stage = "leviq3"
		}
		return fmt.Errorf("%s: %s", stage, h.cancel_reason)
	}
	return nil
}

func (h *LeviQ3Helper) leviq3_wait(ctx context.Context, phase string) error {
	if err := h.check_cancel(ctx, phase); err != nil {
		return err
	}
	h.status.Infof("leviq3_wait: %s", phase)
	if h.motion != nil {
		return h.motion.Sleep(ctx, 200*time.Millisecond)
	}
	return nil
}

func (h *LeviQ3Helper) leviq_set_zero(ctx context.Context) error {
	if err := h.check_cancel(ctx, "leviq_set_zero"); err != nil {
		return err
	}
	h.temperature_compensation = 0
	h.current_zoffset = 0
	h.saved_config.SavedZOffset = 0
	if err := h.leviq_sync_zoffset(ctx, 0); err != nil {
		return err
	}
	if h.motion != nil {
		return h.motion.Set_gcode_offset(ctx, 0)
	}
	return nil
}

func (h *LeviQ3Helper) CMD_LEVIQ3_TEMP_OFFSET(ctx context.Context) error {
	if err := h.check_cancel(ctx, "CMD_LEVIQ3_TEMP_OFFSET"); err != nil {
		return err
	}
	h.temperature_compensation = 0
	h.saved_config.LastCompensationTemp = 0
	h.saved_config.LastTempCompensation = 0
	h.saved_config.LastAutoZOffsetRaw = 0
	h.saved_config.LastAutoZOffset = 0
	h.saved_config.LastAutoZOffsetSamples = nil
	h.last_probe_errors = [3]float64{}
	h.last_probe_samples = [3]float64{}
	h.leviq_scratch_end()
	return h.leviq3_wait(ctx, "temperature offset reset")
}

func (h *LeviQ3Helper) get_auto_zoffset_temperature(ctx context.Context) (float64, error) {
	if h.motion == nil {
		return 0, nil
	}
	if source, ok := h.motion.(AutoZOffsetTemperatureSource); ok {
		return source.GetAutoZOffsetTemperature(ctx)
	}
	return h.motion.GetHotbedTemp(ctx)
}

func (h *LeviQ3Helper) LEVIQ_auto_zoffset(ctx context.Context) (float64, error) {
	if err := h.check_cancel(ctx, "LEVIQ_auto_zoffset"); err != nil {
		return 0, err
	}
	if h.motion == nil {
		return 0, nil
	}
	temperatureSample, err := h.get_auto_zoffset_temperature(ctx)
	if err != nil {
		return 0, err
	}
	if temperatureSample <= 0 {
		h.temperature_compensation = 0
		return 0, nil
	}

	tempCompensation := (temperatureSample - auto_zoffset_reference_temp) * (h.expansion_factor / auto_zoffset_temperature_span)
	tempCompensation = ComputeLeviQ3TemperatureCompensation(temperatureSample, h.expansion_factor)
	h.temperature_compensation = tempCompensation
	h.saved_config.LastHotbedTemp = temperatureSample
	h.saved_config.LastCompensationTemp = temperatureSample
	h.saved_config.LastTempCompensation = tempCompensation
	h.status.Infof("LEVIQ_auto_zoffset: temp=%.2f expansion_factor=%.4f compensation=%.4f", temperatureSample, h.expansion_factor, tempCompensation)
	return tempCompensation, nil
}

func (h *LeviQ3Helper) measure_auto_zoffset_sample(ctx context.Context) (float64, error) {
	if err := h.check_cancel(ctx, "CMD_LEVIQ3_auto_zoffset sample"); err != nil {
		return 0, err
	}
	if h.motion != nil {
		if source, ok := h.motion.(AutoZOffsetMeasurementSource); ok {
			return source.MeasureAutoZOffset(ctx, h)
		}
	}
	mesh := h.probed_matrix
	if mesh == nil {
		mesh = h.saved_config.SavedMesh
	}
	if mesh != nil && len(mesh.points) > 0 {
		avg := mesh.Average()
		if probe_point_range(mesh.points) > h.scratch_sensitivity {
			h.leviq_scratch_notice()
		}
		sample := h.rough_platform_offset.Z
		if sample == 0 {
			sample = avg
		} else if avg != 0 {
			sample = (sample + avg) / 2
		}
		return sample, nil
	}
	return h.rough_platform_offset.Z, nil
}

func (h *LeviQ3Helper) record_auto_zoffset_sample(index int, value float64) {
	if index >= 0 && index < len(h.last_probe_samples) {
		h.last_probe_samples[index] = value
	}
	if index > 0 && index-1 < len(h.last_probe_errors) {
		h.last_probe_errors[index-1] = math.Abs(h.last_probe_samples[index] - h.last_probe_samples[index-1])
	}
}

func (h *LeviQ3Helper) validate_auto_zoffset_samples(ctx context.Context) (float64, []float64, error) {
	maxAttempts := clamp_int(h.auto_zoffset_retry_count+1, 2, 3)
	samples := make([]float64, 0, maxAttempts)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		sample, err := h.measure_auto_zoffset_sample(ctx)
		if err != nil {
			return 0, samples, err
		}
		samples = append(samples, sample)
		h.record_auto_zoffset_sample(attempt, sample)
		if len(samples) < 2 {
			continue
		}
		diff := math.Abs(samples[len(samples)-1] - samples[len(samples)-2])
		if diff <= h.max_diff {
			candidate := average_float64s(samples[len(samples)-2:])
			if range_float64s(samples) > h.noise_diff {
				h.leviq_scratch_notice()
			}
			return candidate, samples, nil
		}
		if attempt == 1 {
			h.status.Infof("CMD_LEVIQ3_auto_zoffset: first diff too big,re-auto_zoffset start")
		} else if attempt == 2 {
			h.status.Infof("CMD_LEVIQ3_auto_zoffset: second diff too big,re-auto_zoffset start")
		}
	}
	if range_float64s(samples) > h.noise_diff {
		h.leviq_scratch_notice()
	}
	return median_float64s(samples), samples, nil
}

func (h *LeviQ3Helper) clamp_auto_zoffset(candidate float64) float64 {
	base := h.current_zoffset
	if base == 0 {
		base = h.saved_config.SavedZOffset
	}
	if base != 0 && candidate > base+default_auto_zoffset_step_limit {
		return base + default_auto_zoffset_step_limit
	}
	return candidate
}

func (h *LeviQ3Helper) compute_auto_zoffset(ctx context.Context) (float64, error) {
	compensation, err := h.LEVIQ_auto_zoffset(ctx)
	if err != nil {
		return 0, err
	}
	raw, samples, err := h.validate_auto_zoffset_samples(ctx)
	if err != nil {
		return 0, err
	}
	candidate := h.zoffset_base + raw + compensation
	candidate = h.clamp_auto_zoffset(candidate)
	h.last_valid_auto_zoffset = candidate
	h.saved_config.LastAutoZOffsetRaw = raw
	h.saved_config.LastAutoZOffset = candidate
	h.saved_config.LastAutoZOffsetSamples = append([]float64(nil), samples...)
	h.status.Infof("CMD_LEVIQ3_auto_zoffset: raw=%.4f compensation=%.4f candidate=%.4f", raw, compensation, candidate)
	return candidate, nil
}

func (h *LeviQ3Helper) CMD_LEVIQ3_auto_zoffset_ON_OFF(enable bool) {
	h.auto_zoffset_on_off = enable
	if !enable {
		h.temperature_compensation = 0
	}
	h.status.Infof("leviQ3 auto_zoffset_on_off: %t", enable)
}

func (h *LeviQ3Helper) leviq_sync_zoffset(ctx context.Context, zoffset float64) error {
	if err := h.check_cancel(ctx, "leviq:sync_zoffset"); err != nil {
		return err
	}
	if h.motion != nil {
		if target, ok := h.motion.(ProbeOffsetSyncTarget); ok {
			if err := target.Z_offset_apply_probe(ctx, zoffset); err != nil {
				return err
			}
		}
		if target, ok := h.motion.(AbsoluteProbeOffsetSyncTarget); ok {
			if err := target.Z_offset_apply_probe_absolute(ctx, zoffset); err != nil {
				return err
			}
		}
		if target, ok := h.motion.(MeshOffsetSyncTarget); ok {
			if err := target.Set_mesh_offsets(ctx, XYZ{X: h.mesh_offsets.X, Y: h.mesh_offsets.Y, Z: zoffset}); err != nil {
				return err
			}
		}
		if target, ok := h.motion.(RailsOffsetSyncTarget); ok {
			if err := target.Set_rails_z_offset(ctx, zoffset); err != nil {
				return err
			}
		}
	}
	h.saved_config.LastAppliedOffsets = h.Get_offsets()
	return nil
}

func (h *LeviQ3Helper) CMD_LEVIQ3_auto_zoffset(ctx context.Context) error {
	if err := h.check_cancel(ctx, "CMD_LEVIQ3_auto_zoffset"); err != nil {
		return err
	}
	if !h.auto_zoffset_on_off {
		return nil
	}
	candidate, err := h.compute_auto_zoffset(ctx)
	if err != nil {
		h.CancelEvent(err.Error())
		return err
	}
	return h.LEVIQ3_set_zoffset(ctx, candidate)
}

func (h *LeviQ3Helper) LEVIQ3_set_zoffset(ctx context.Context, zoffset float64) error {
	if err := h.check_cancel(ctx, "LEVIQ3_set_zoffset"); err != nil {
		return err
	}
	h.current_zoffset = zoffset
	h.saved_config.SavedZOffset = zoffset
	h.saved_config.LastAppliedOffsets = h.Get_offsets()
	if err := h.leviq_sync_zoffset(ctx, zoffset); err != nil {
		return err
	}
	if h.motion != nil {
		return h.motion.Set_gcode_offset(ctx, zoffset)
	}
	return nil
}

func (h *LeviQ3Helper) preheat_target_temperature() float64 {
	targetBedTemp := math.Max(h.temperature.defaultLevelingSwipingTemp, h.temperature.bed_temp)
	if h.auto_zoffset_on_off {
		targetBedTemp = math.Max(targetBedTemp, h.temperature.first_layer_bed_temperature)
	}
	return targetBedTemp
}

func (h *LeviQ3Helper) ensure_preheat_target(ctx context.Context, target float64) error {
	if h.motion == nil {
		return nil
	}
	if err := h.check_cancel(ctx, "CMD_LEVIQ3_PREHEATING"); err != nil {
		return err
	}
	if err := h.motion.Set_bed_temperature(ctx, target); err != nil {
		return err
	}
	deadline := time.Now().Add(h.temperature.HotbedTemperatureTimeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("CMD_LEVIQ3_PREHEATING: temp dropped below target-%.0f, waiting again timed out", h.preheat_recheck_margin)
		}
		if err := h.motion.Wait_for_temperature(ctx, target, remaining); err != nil {
			return err
		}
		current, err := h.motion.GetHotbedTemp(ctx)
		if err != nil {
			return err
		}
		h.saved_config.LastHotbedTemp = current
		if current >= target-h.preheat_recheck_margin {
			return nil
		}
		h.status.Infof("CMD_LEVIQ3_PREHEATING: temp dropped below target-%.0f, waiting again...", h.preheat_recheck_margin)
	}
}

func (h *LeviQ3Helper) CMD_LEVIQ3_PREHEATING(ctx context.Context, arg string) error {
	if err := h.check_cancel(ctx, "CMD_LEVIQ3_PREHEATING"); err != nil {
		return err
	}
	if strings.EqualFold(arg, "enable") {
		h.preheat_leveling = true
	}
	if strings.EqualFold(arg, "disable") {
		h.preheat_leveling = false
	}
	if !h.preheat_leveling || h.motion == nil {
		return nil
	}
	targetBedTemp := h.preheat_target_temperature()
	h.status.Infof("CMD_LEVIQ3_PREHEATING: target bed temp %.2f", targetBedTemp)
	return h.ensure_preheat_target(ctx, targetBedTemp)
}

func (h *LeviQ3Helper) leviq_scratch_notice() {
	h.is_scratch_notice = true
	h.status.Infof("leviq_scratch_notice")
}

func (h *LeviQ3Helper) leviq_scratch_start() {
	h.is_scratch_notice = true
	h.status.Infof("leviq_scratch_start")
}

func (h *LeviQ3Helper) leviq_scratch_end() {
	h.is_scratch_notice = false
	h.status.Infof("leviq_scratch_end")
}

func (h *LeviQ3Helper) CMD_G9113(ctx context.Context) error {
	if err := h.check_cancel(ctx, "CMD_G9113"); err != nil {
		return err
	}
	h.debug_g9113 = true
	h.leviq_scratch_notice()
	return h.leviq3_wait(ctx, "CMD_G9113")
}

func (h *LeviQ3Helper) resolve_wipe_sequence(ctx context.Context) ([]XYZ, error) {
	if h.motion != nil {
		if provider, ok := h.motion.(LeviQ3WipeSequenceProvider); ok {
			sequence, err := provider.LeviQ3WipeSequence(ctx, h)
			if err != nil {
				return nil, err
			}
			if len(sequence) > 0 {
				return sequence, nil
			}
		}
	}
	if len(h.wiping_sequence) == 0 {
		h.wiping_sequence = h.build_wiping_sequence()
	}
	return append([]XYZ(nil), h.wiping_sequence...), nil
}

func (h *LeviQ3Helper) CMD_LEVIQ3_WIPING(ctx context.Context) error {
	if err := h.check_cancel(ctx, "CMD_LEVIQ3_WIPING"); err != nil {
		return err
	}
	if h.motion == nil {
		return nil
	}
	if err := h.CMD_G9113(ctx); err != nil {
		return err
	}
	h.leviq_scratch_start()
	defer func() {
		h.debug_g9113 = false
		h.leviq_scratch_end()
	}()

	if h.preheat_leveling {
		if err := h.ensure_preheat_target(ctx, h.preheat_target_temperature()); err != nil {
			return err
		}
	}
	sequence, err := h.resolve_wipe_sequence(ctx)
	if err != nil {
		return err
	}
	for index, position := range sequence {
		if err := h.check_cancel(ctx, fmt.Sprintf("wipe pass %d", index+1)); err != nil {
			return err
		}
		h.status.Infof("CMD_LEVIQ3_WIPING: pass %d/%d at (%.2f, %.2f, %.2f)", index+1, len(sequence), position.X, position.Y, position.Z)
		if err := h.motion.Wipe_nozzle(ctx, position); err != nil {
			return handErrorLeviq3(fmt.Sprintf("Wipe_nozzle[%d]", index), err)
		}
		if index+1 < len(sequence) {
			if err := h.leviq3_wait(ctx, fmt.Sprintf("wipe pass %d/%d", index+1, len(sequence))); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *LeviQ3Helper) Start_probe(ctx context.Context) error {
	if err := h.check_cancel(ctx, "Start_probe"); err != nil {
		return err
	}
	h.is_homing = true
	h.last_probe_samples = [3]float64{}
	h.last_probe_errors = [3]float64{}
	h.debug_g9113 = false
	if h.motion != nil {
		if err := h.motion.Clear_homing_state(ctx); err != nil {
			return err
		}
	}
	return h.leviq3_wait(ctx, "Start_probe")
}

func (h *LeviQ3Helper) home_axes_generic(ctx context.Context, axes []Axis, params HomingParams) error {
	if err := h.check_cancel(ctx, "home axes"); err != nil {
		return err
	}
	if h.motion == nil || len(axes) == 0 {
		return nil
	}
	if len(axes) == 1 {
		return h.motion.Home_axis(ctx, axes[0], params)
	}
	return h.motion.Home_rails(ctx, axes, params)
}

func (h *LeviQ3Helper) homing_retract_delta() float64 {
	delta := math.Abs(h.homing_params.homing_retract_dist)
	if h.homing_params.homing_positive_dir {
		return -delta
	}
	return delta
}

func (h *LeviQ3Helper) perform_secondary_homing_pass(ctx context.Context, axes []Axis) error {
	if h.motion == nil || len(axes) == 0 {
		return nil
	}
	retract := h.homing_retract_delta()
	for _, axis := range axes {
		if err := h.Homing_move(ctx, axis, retract, h.homing_params.homing_retract_speed); err != nil {
			return err
		}
	}
	second := h.homing_params
	if second.second_homing_speed > 0 {
		second.homing_speed = second.second_homing_speed
	}
	return h.home_axes_generic(ctx, axes, second)
}

func (h *LeviQ3Helper) Home_CartKinematics(ctx context.Context, axes ...Axis) error {
	axes = normalize_axes(axes)
	if err := h.check_cancel(ctx, "Home_CartKinematics"); err != nil {
		return err
	}
	xyAxes := filter_axes(axes, AxisX, AxisY)
	zAxes := filter_axes(axes, AxisZ)
	if len(xyAxes) > 0 {
		if err := h.home_axes_generic(ctx, xyAxes, h.homing_params); err != nil {
			return err
		}
		for retry := 0; retry < h.homing_params.homing_retry_count; retry++ {
			if err := h.perform_secondary_homing_pass(ctx, xyAxes); err != nil {
				return err
			}
		}
	}
	if len(zAxes) > 0 {
		if h.homing_pos_xy.X != 0 || h.homing_pos_xy.Y != 0 {
			if err := h.Homing_move(ctx, AxisX, h.homing_pos_xy.X, h.homing_params.homing_speed); err != nil {
				return err
			}
			if err := h.Homing_move(ctx, AxisY, h.homing_pos_xy.Y, h.homing_params.homing_speed); err != nil {
				return err
			}
		}
		if err := h.home_axes_generic(ctx, zAxes, h.homing_params); err != nil {
			return err
		}
		for retry := 0; retry < h.homing_params.homing_retry_count; retry++ {
			if err := h.perform_secondary_homing_pass(ctx, zAxes); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *LeviQ3Helper) Home_CoreXyKinematics(ctx context.Context, axes ...Axis) error {
	axes = normalize_axes(axes)
	if err := h.check_cancel(ctx, "Home_CoreXyKinematics"); err != nil {
		return err
	}
	xyRequested := contains_axis(axes, AxisX) || contains_axis(axes, AxisY)
	if xyRequested {
		xyAxes := []Axis{AxisX, AxisY}
		if err := h.home_axes_generic(ctx, xyAxes, h.homing_params); err != nil {
			return err
		}
		for retry := 0; retry < h.homing_params.homing_retry_count; retry++ {
			if err := h.perform_secondary_homing_pass(ctx, xyAxes); err != nil {
				return err
			}
		}
	}
	if contains_axis(axes, AxisZ) {
		if h.homing_pos_xy.X != 0 || h.homing_pos_xy.Y != 0 {
			if err := h.Homing_move(ctx, AxisX, h.homing_pos_xy.X, h.homing_params.homing_speed); err != nil {
				return err
			}
			if err := h.Homing_move(ctx, AxisY, h.homing_pos_xy.Y, h.homing_params.homing_speed); err != nil {
				return err
			}
		}
		if err := h.home_axes_generic(ctx, []Axis{AxisZ}, h.homing_params); err != nil {
			return err
		}
		for retry := 0; retry < h.homing_params.homing_retry_count; retry++ {
			if err := h.perform_secondary_homing_pass(ctx, []Axis{AxisZ}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *LeviQ3Helper) Home_axis(ctx context.Context, axis Axis) error {
	return h.Home_rails(ctx, axis)
}

func (h *LeviQ3Helper) Home_rails(ctx context.Context, axes ...Axis) error {
	axes = normalize_axes(axes)
	if len(axes) == 0 {
		return nil
	}
	switch strings.ToLower(h.kinematics) {
	case "corexy", "kinematics.corexy":
		return h.Home_CoreXyKinematics(ctx, axes...)
	default:
		return h.Home_CartKinematics(ctx, axes...)
	}
}

func (h *LeviQ3Helper) Homing_move(ctx context.Context, axis Axis, target float64, speed float64) error {
	if err := h.check_cancel(ctx, "Homing_move"); err != nil {
		return err
	}
	if h.motion != nil {
		return h.motion.Homing_move(ctx, axis, target, speed)
	}
	return nil
}

func (h *LeviQ3Helper) CMD_LEVIQ3_PROBE(ctx context.Context) (*BedMesh, error) {
	if err := h.check_cancel(ctx, "CMD_LEVIQ3_PROBE"); err != nil {
		return nil, err
	}
	if err := h.Start_probe(ctx); err != nil {
		return nil, err
	}
	defer func() {
		h.is_homing = false
	}()
	if err := h.Home_rails(ctx, AxisX, AxisY); err != nil {
		return nil, err
	}
	if h.motion == nil {
		return nil, errors.New("no motion controller available for probing")
	}

	points := make([]ProbePoint, 0, len(h.probe_points))
	for i, position := range h.probe_points {
		if err := h.check_cancel(ctx, fmt.Sprintf("probe point %d/%d", i+1, len(h.probe_points))); err != nil {
			return nil, err
		}
		if err := h.leviq3_wait(ctx, fmt.Sprintf("probe point %d/%d", i+1, len(h.probe_points))); err != nil {
			return nil, err
		}
		if err := h.motion.Lower_probe(ctx); err != nil {
			h.CancelEvent(err.Error())
			return nil, handErrorLeviq3("Lower_probe", err)
		}
		measuredZ, err := h.motion.Run_probe(ctx, position)
		if err != nil {
			h.CancelEvent(err.Error())
			return nil, handErrorLeviq3("Run_probe", err)
		}
		if err := h.motion.Raise_probe(ctx); err != nil {
			h.CancelEvent(err.Error())
			return nil, handErrorLeviq3("Raise_probe", err)
		}
		if i < len(h.last_probe_samples) {
			h.last_probe_samples[i] = measuredZ
		}
		if len(points) > 0 {
			diff := math.Abs(measuredZ - points[len(points)-1].MeasuredZ)
			if i-1 < len(h.last_probe_errors) {
				h.last_probe_errors[i-1] = diff
			}
			if diff > h.scratch_sensitivity {
				h.leviq_scratch_notice()
			}
		}
		points = append(points, ProbePoint{
			Index:     i,
			Position:  position,
			MeasuredZ: measuredZ,
			Valid:     true,
		})
	}

	mesh := h.Build_mesh(points)
	h.Set_mesh(mesh)
	if err := h.motion.Save_mesh(ctx, mesh); err != nil {
		return nil, err
	}
	return mesh, nil
}

func (h *LeviQ3Helper) CMD_LEVIQ3(ctx context.Context) (*BedMesh, error) {
	h.clear_cancel()
	if err := h.handle_ready(ctx); err != nil {
		return nil, err
	}
	if err := h.CMD_LEVIQ3_PREHEATING(ctx, "enable"); err != nil {
		h.CancelEvent(err.Error())
		return nil, handErrorLeviq3("CMD_LEVIQ3_PREHEATING", err)
	}
	if err := h.CMD_LEVIQ3_WIPING(ctx); err != nil {
		h.CancelEvent(err.Error())
		return nil, handErrorLeviq3("CMD_LEVIQ3_WIPING", err)
	}
	mesh, err := h.CMD_LEVIQ3_PROBE(ctx)
	if err != nil {
		h.CancelEvent(err.Error())
		return nil, handErrorLeviq3("CMD_LEVIQ3_PROBE", err)
	}
	if err := h.CMD_LEVIQ3_auto_zoffset(ctx); err != nil {
		h.CancelEvent(err.Error())
		return nil, handErrorLeviq3("CMD_LEVIQ3_auto_zoffset", err)
	}

	h.saved_config.SavedMesh = mesh
	h.saved_config.LastRun = time.Now()
	h.is_homing = false
	h.ready = true
	return mesh, nil
}

func (h *LeviQ3Helper) Build_mesh(points []ProbePoint) *BedMesh {
	mesh := NewBedMesh(h.mesh_config.mesh_min, h.mesh_config.mesh_max, h.mesh_config.probe_count, "CMD_LEVIQ3_PROBE")
	mesh.points = append(mesh.points, points...)

	xValues := unique_sorted_x(points)
	yValues := unique_sorted_y(points)
	if len(xValues) == mesh.count.X && len(yValues) == mesh.count.Y {
		for _, point := range points {
			x := index_of_float(xValues, point.Position.X)
			y := index_of_float(yValues, point.Position.Y)
			if x >= 0 && y >= 0 {
				mesh.Set(x, y, point.MeasuredZ)
			}
		}
	}
	return mesh
}

func (h *LeviQ3Helper) Get_mesh() *BedMesh {
	return h.probed_matrix
}

func (h *LeviQ3Helper) Set_mesh(mesh *BedMesh) {
	h.probed_matrix = mesh
	if mesh != nil {
		avg := mesh.Average()
		if avg != 0 {
			h.rough_platform_offset.Z = avg
		}
	}
}

func (h *LeviQ3Helper) Get_mesh_matrix() [][]float64 {
	if h.probed_matrix == nil {
		return nil
	}
	return h.probed_matrix.CloneMatrix()
}

func (h *LeviQ3Helper) Get_probed_matrix() [][]float64 {
	return h.Get_mesh_matrix()
}

func (h *LeviQ3Helper) Print_mesh() string {
	if h.probed_matrix == nil {
		return "<no mesh>"
	}
	return h.probed_matrix.Print_probed_matrix()
}

func (h *LeviQ3Helper) Update_probe_points(points []XY) {
	h.probe_points = append(h.probe_points[:0], points...)
}

func (h *LeviQ3Helper) Get_mesh_params() MeshConfig {
	return h.mesh_config
}

func (h *LeviQ3Helper) Set_mesh_offsets(offsets XYZ) {
	h.mesh_offsets = offsets
	h.mesh_config.mesh_offsets = offsets
}

func (h *LeviQ3Helper) Get_offsets() XYZ {
	if !h.use_xy_offsets {
		return XYZ{Z: h.current_zoffset}
	}
	return XYZ{
		X: h.probe_offsets.X + h.mesh_offsets.X + h.platform_offset.X,
		Y: h.probe_offsets.Y + h.mesh_offsets.Y + h.platform_offset.Y,
		Z: h.current_zoffset + h.probe_offsets.Z + h.mesh_offsets.Z + h.platform_offset.Z,
	}
}

func (h *LeviQ3Helper) CMD_LEVIQ3_HELP() string {
	text := strings.Join([]string{
		"LEVIQ3: runs full preheat, wipe, probe, and auto z-offset recovery",
		"LEVIQ3 PREHEATING: prepares the bed for leveling and re-checks temperature stability",
		"LEVIQ3 WIPING: runs the configured wipe sequence with scratch monitoring",
		"LEVIQ3 PROBE: homes, probes the platform, and builds the recovered mesh",
		"G9113: arms the recovered LeviQ scratch/debug notice path",
	}, "\n")
	h.status.Infof(text)
	return text
}

func (h *LeviQ3Helper) CMD_LEVIQ3_PREHEATING_HELP() string {
	text := "LEVIQ3 PREHEATING keeps the bed at the leveling target and waits again if temperature falls below target-5."
	h.status.Infof(text)
	return text
}

func (h *LeviQ3Helper) CMD_LEVIQ3_WIPING_HELP() string {
	text := "LEVIQ3 WIPING executes the recovered wipe sequence, keeps scratch notice state active, and respects cancellation."
	h.status.Infof(text)
	return text
}

func (h *LeviQ3Helper) CMD_LEVIQ3_PROBE_HELP() string {
	text := "LEVIQ3 PROBE homes according to the configured kinematics, probes each LeviQ point, tracks probe noise, and updates the recovered mesh state."
	h.status.Infof(text)
	return text
}

func (h *LeviQ3Helper) build_probe_points() []XY {
	count := h.mesh_config.probe_count
	if count.X < 2 {
		count.X = 4
	}
	if count.Y < 2 {
		count.Y = 4
	}

	points := make([]XY, 0, count.X*count.Y)
	xStep := 0.0
	yStep := 0.0
	if count.X > 1 {
		xStep = (h.mesh_config.mesh_max.X - h.mesh_config.mesh_min.X) / float64(count.X-1)
	}
	if count.Y > 1 {
		yStep = (h.mesh_config.mesh_max.Y - h.mesh_config.mesh_min.Y) / float64(count.Y-1)
	}
	for y := 0; y < count.Y; y++ {
		for x := 0; x < count.X; x++ {
			points = append(points, XY{
				X: h.mesh_config.mesh_min.X + float64(x)*xStep,
				Y: h.mesh_config.mesh_min.Y + float64(y)*yStep,
			})
		}
	}
	return points
}

func (h *LeviQ3Helper) build_wiping_sequence() []XYZ {
	base := h.wiping_position
	left := XYZ{
		X: clip_float64(base.X-default_wipe_travel_step, 0, h.platform_x_size),
		Y: base.Y,
		Z: base.Z,
	}
	right := XYZ{
		X: clip_float64(base.X+default_wipe_travel_step, 0, h.platform_x_size),
		Y: base.Y,
		Z: base.Z,
	}
	return []XYZ{base, left, right}
}

func handErrorLeviq3(stage string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", stage, err)
}

func xy_from_slice(values []float64) XY {
	xy := XY{}
	if len(values) > 0 {
		xy.X = values[0]
	}
	if len(values) > 1 {
		xy.Y = values[1]
	}
	return xy
}

func xyz_from_slice(values []float64) XYZ {
	xyz := XYZ{}
	if len(values) > 0 {
		xyz.X = values[0]
	}
	if len(values) > 1 {
		xyz.Y = values[1]
	}
	if len(values) > 2 {
		xyz.Z = values[2]
	}
	return xyz
}

func xy_count_from_slice(values []float64, fallback XYCount) XYCount {
	count := fallback
	if len(values) > 0 {
		count.X = int(values[0])
	}
	if len(values) > 1 {
		count.Y = int(values[1])
	}
	if count.X < 1 {
		count.X = fallback.X
	}
	if count.Y < 1 {
		count.Y = fallback.Y
	}
	return count
}

func xy_pairs_from_slice(values []float64) []XY {
	if len(values) < 2 {
		return nil
	}
	points := make([]XY, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		points = append(points, XY{X: values[i], Y: values[i+1]})
	}
	return points
}

func xyz_triples_from_slice(values []float64) []XYZ {
	if len(values) < 3 {
		return nil
	}
	points := make([]XYZ, 0, len(values)/3)
	for i := 0; i+2 < len(values); i += 3 {
		points = append(points, XYZ{X: values[i], Y: values[i+1], Z: values[i+2]})
	}
	return points
}

func unique_sorted_x(points []ProbePoint) []float64 {
	values := make([]float64, 0, len(points))
	seen := map[float64]struct{}{}
	for _, point := range points {
		if _, ok := seen[point.Position.X]; ok {
			continue
		}
		seen[point.Position.X] = struct{}{}
		values = append(values, point.Position.X)
	}
	sort.Float64s(values)
	return values
}

func unique_sorted_y(points []ProbePoint) []float64 {
	values := make([]float64, 0, len(points))
	seen := map[float64]struct{}{}
	for _, point := range points {
		if _, ok := seen[point.Position.Y]; ok {
			continue
		}
		seen[point.Position.Y] = struct{}{}
		values = append(values, point.Position.Y)
	}
	sort.Float64s(values)
	return values
}

func index_of_float(values []float64, target float64) int {
	for i, value := range values {
		if math.Abs(value-target) < 1e-9 {
			return i
		}
	}
	return -1
}

func normalize_axes(axes []Axis) []Axis {
	ordered := []Axis{AxisX, AxisY, AxisZ}
	seen := map[Axis]struct{}{}
	for _, axis := range axes {
		seen[Axis(strings.ToLower(string(axis)))] = struct{}{}
	}
	out := make([]Axis, 0, len(seen))
	for _, axis := range ordered {
		if _, ok := seen[axis]; ok {
			out = append(out, axis)
		}
	}
	for _, axis := range axes {
		normalized := Axis(strings.ToLower(string(axis)))
		if _, ok := seen[normalized]; ok && !contains_axis(out, normalized) {
			out = append(out, normalized)
		}
	}
	return out
}

func filter_axes(axes []Axis, wanted ...Axis) []Axis {
	out := make([]Axis, 0, len(axes))
	for _, axis := range axes {
		for _, candidate := range wanted {
			if axis == candidate {
				out = append(out, axis)
				break
			}
		}
	}
	return out
}

func contains_axis(axes []Axis, axis Axis) bool {
	for _, current := range axes {
		if current == axis {
			return true
		}
	}
	return false
}

func average_float64s(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func median_float64s(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func range_float64s(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minValue := values[0]
	maxValue := values[0]
	for _, value := range values[1:] {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue - minValue
}

func probe_point_range(points []ProbePoint) float64 {
	values := make([]float64, 0, len(points))
	for _, point := range points {
		if point.Valid {
			values = append(values, point.MeasuredZ)
		}
	}
	return range_float64s(values)
}

func add_xyz(a, b XYZ) XYZ {
	return XYZ{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z}
}

func clip_float64(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clamp_int(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	default:
		return 0, false
	}
}

func toFloat64Slice(v any) ([]float64, bool) {
	switch x := v.(type) {
	case []float64:
		return append([]float64(nil), x...), true
	case []float32:
		out := make([]float64, len(x))
		for i, value := range x {
			out[i] = float64(value)
		}
		return out, true
	case []int:
		out := make([]float64, len(x))
		for i, value := range x {
			out[i] = float64(value)
		}
		return out, true
	case []any:
		out := make([]float64, 0, len(x))
		for _, value := range x {
			f, ok := toFloat64(value)
			if !ok {
				return nil, false
			}
			out = append(out, f)
		}
		return out, true
	default:
		return nil, false
	}
}
