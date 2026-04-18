package bedmesh

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

const ProfileVersion = 1

var ProfileOptionKinds = map[string]reflect.Kind{
	"min_x": reflect.Float64, "max_x": reflect.Float64, "min_y": reflect.Float64, "max_y": reflect.Float64,
	"x_count": reflect.Int, "y_count": reflect.Int, "mesh_x_pps": reflect.Int, "mesh_y_pps": reflect.Int,
	"algo": reflect.String, "tension": reflect.Float64,
}

type ProfileRecord struct {
	ProbedMatrix [][]float64
	MeshParams   map[string]interface{}
}

type StoredProfileInput struct {
	Name       string
	Version    int
	PointsData interface{}
	MeshParams map[string]interface{}
}

type IncompatibleProfile struct {
	Name    string
	Version int
}

type StoredProfileStore struct {
	Profiles     map[string]interface{}
	Incompatible []IncompatibleProfile
}

type ProfileManager struct {
	profiles       map[string]interface{}
	currentProfile string
	incompatible   []IncompatibleProfile
}

var (
	ErrUnknownProfile         = errors.New("bed_mesh: unknown profile")
	ErrProfileSaveWithoutMesh = errors.New("bed_mesh: cannot save profile without mesh")
)

type MeshStatus struct {
	ProfileName  string
	MeshMin      []float64
	MeshMax      []float64
	ProbedMatrix [][]float64
	MeshMatrix   [][]float64
	Profiles     map[string]interface{}
}

func NewProfileRecord(probedMatrix [][]float64, meshParams map[string]interface{}) ProfileRecord {
	return ProfileRecord{
		ProbedMatrix: NormalizeProbedMatrixType(probedMatrix),
		MeshParams:   cloneMeshParams(meshParams),
	}
}

func NewStoredProfileRecord(pointsData interface{}, meshParams map[string]interface{}) ProfileRecord {
	return ProfileRecord{
		ProbedMatrix: NormalizeProbedMatrixType(pointsData),
		MeshParams:   cloneMeshParams(meshParams),
	}
}

func DecodeStoredProfileStore(inputs []StoredProfileInput) StoredProfileStore {
	store := StoredProfileStore{
		Profiles:     map[string]interface{}{},
		Incompatible: []IncompatibleProfile{},
	}
	for _, input := range inputs {
		if input.Version != ProfileVersion {
			store.Incompatible = append(store.Incompatible, IncompatibleProfile{Name: input.Name, Version: input.Version})
			continue
		}
		store.Profiles[input.Name] = NewStoredProfileRecord(input.PointsData, input.MeshParams).StatusEntry()
	}
	return store
}

func CloneIncompatibleProfiles(profiles []IncompatibleProfile) []IncompatibleProfile {
	if len(profiles) == 0 {
		return nil
	}
	cloned := make([]IncompatibleProfile, len(profiles))
	copy(cloned, profiles)
	return cloned
}

func NewProfileManager(inputs []StoredProfileInput) *ProfileManager {
	store := DecodeStoredProfileStore(inputs)
	return &ProfileManager{
		profiles:     store.Profiles,
		incompatible: CloneIncompatibleProfiles(store.Incompatible),
	}
}

func (manager *ProfileManager) Profiles() map[string]interface{} {
	return CloneProfileStore(manager.profiles)
}

func (manager *ProfileManager) CurrentProfile() string {
	return manager.currentProfile
}

func (manager *ProfileManager) IncompatibleProfiles() []IncompatibleProfile {
	return CloneIncompatibleProfiles(manager.incompatible)
}

func (manager *ProfileManager) BuildStatus(mesh *ZMesh) MeshStatus {
	return BuildMeshStatus(manager.currentProfile, manager.profiles, mesh)
}

func (manager *ProfileManager) SaveProfile(profileName string, mesh *ZMesh) (ProfileRecord, error) {
	if mesh == nil {
		return ProfileRecord{}, ErrProfileSaveWithoutMesh
	}
	record := NewProfileRecord(mesh.Get_probed_matrix(), mesh.Get_mesh_params())
	manager.profiles = WithStoredProfile(manager.profiles, profileName, record)
	manager.currentProfile = profileName
	return record, nil
}

func (manager *ProfileManager) LoadProfile(profileName string) (*ZMesh, error) {
	profile, ok := manager.profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("%w [%s]", ErrUnknownProfile, profileName)
	}
	entry, ok := profile.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("bed_mesh: invalid stored profile [%s]", profileName)
	}
	meshParams, ok := entry["mesh_params"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("bed_mesh: invalid mesh params for profile [%s]", profileName)
	}
	record := NewStoredProfileRecord(entry["points"], meshParams)
	zMesh, err := RestoreZMeshFromProfile(record)
	if err != nil {
		return nil, err
	}
	manager.currentProfile = profileName
	return zMesh, nil
}

func (manager *ProfileManager) RemoveProfile(profileName string) bool {
	if _, ok := manager.profiles[profileName]; !ok {
		return false
	}
	manager.profiles = WithoutStoredProfile(manager.profiles, profileName)
	if manager.currentProfile == profileName {
		manager.currentProfile = ""
	}
	return true
}

func CloneProfileStore(profiles map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(profiles))
	for name, value := range profiles {
		entry, ok := value.(map[string]interface{})
		if !ok || entry == nil {
			cloned[name] = value
			continue
		}
		points := NormalizeProbedMatrixType(entry["points"])
		meshParams, _ := entry["mesh_params"].(map[string]interface{})
		cloned[name] = map[string]interface{}{
			"points":      points,
			"mesh_params": cloneMeshParams(meshParams),
		}
	}
	return cloned
}

func WithStoredProfile(profiles map[string]interface{}, profileName string, record ProfileRecord) map[string]interface{} {
	cloned := CloneProfileStore(profiles)
	cloned[profileName] = record.StatusEntry()
	return cloned
}

func WithoutStoredProfile(profiles map[string]interface{}, profileName string) map[string]interface{} {
	cloned := CloneProfileStore(profiles)
	delete(cloned, profileName)
	return cloned
}

func BuildMeshStatus(currentProfile string, profiles map[string]interface{}, mesh *ZMesh) MeshStatus {
	status := MeshStatus{
		MeshMin:      []float64{0.0, 0.0},
		MeshMax:      []float64{0.0, 0.0},
		ProbedMatrix: [][]float64{},
		MeshMatrix:   [][]float64{},
		Profiles:     CloneProfileStore(profiles),
	}
	if mesh == nil {
		return status
	}
	params := cloneMeshParams(mesh.Get_mesh_params())
	status.ProfileName = currentProfile
	status.MeshMin = []float64{params["min_x"].(float64), params["min_y"].(float64)}
	status.MeshMax = []float64{params["max_x"].(float64), params["max_y"].(float64)}
	status.ProbedMatrix = mesh.Get_probed_matrix()
	status.MeshMatrix = mesh.Get_mesh_matrix()
	return status
}

func (status MeshStatus) AsMap() map[string]interface{} {
	return map[string]interface{}{
		"profile_name":  status.ProfileName,
		"mesh_min":      append([]float64(nil), status.MeshMin...),
		"mesh_max":      append([]float64(nil), status.MeshMax...),
		"probed_matrix": NormalizeProbedMatrixType(status.ProbedMatrix),
		"mesh_matrix":   NormalizeProbedMatrixType(status.MeshMatrix),
		"profiles":      CloneProfileStore(status.Profiles),
	}
}

func MarshalMeshMap(mesh *ZMesh) ([]byte, error) {
	if mesh == nil {
		return json.Marshal(map[string]interface{}{})
	}
	params := cloneMeshParams(mesh.Get_mesh_params())
	payload := map[string]interface{}{
		"mesh_min":    []float64{params["min_x"].(float64), params["min_y"].(float64)},
		"mesh_max":    []float64{params["max_x"].(float64), params["max_y"].(float64)},
		"z_positions": mesh.Get_probed_matrix(),
	}
	return json.Marshal(payload)
}

func (record ProfileRecord) ConfigValues() map[string]string {
	values := map[string]string{
		"version": strconv.Itoa(ProfileVersion),
		"points":  FormatProbedMatrixForConfig(record.ProbedMatrix),
	}
	for key, value := range record.MeshParams {
		switch typed := value.(type) {
		case string:
			values[key] = typed
		case int:
			values[key] = strconv.Itoa(typed)
		case float64:
			values[key] = strconv.FormatFloat(typed, 'f', -1, 64)
		}
	}
	return values
}

func (record ProfileRecord) StatusEntry() map[string]interface{} {
	return map[string]interface{}{
		"points":      NormalizeProbedMatrixType(record.ProbedMatrix),
		"mesh_params": cloneMeshParams(record.MeshParams),
	}
}

func RestoreZMeshFromProfile(record ProfileRecord) (*ZMesh, error) {
	zMesh := NewZMesh(cloneMeshParams(record.MeshParams))
	if err := BuildMeshSafely(zMesh, NormalizeProbedMatrixType(record.ProbedMatrix)); err != nil {
		return nil, err
	}
	return zMesh, nil
}
