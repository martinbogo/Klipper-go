package bedmesh

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestProfileRecordConfigValuesAndRestoreZMesh(t *testing.T) {
	params := testMeshParams()
	matrix := [][]float64{{0, 1}, {2, 3}}
	record := NewProfileRecord(matrix, params)

	values := record.ConfigValues()
	if values["version"] != "1" {
		t.Fatalf("version = %q, want 1", values["version"])
	}
	if values["points"] != "0.000000, 1.000000\n2.000000, 3.000000" {
		t.Fatalf("points = %q", values["points"])
	}

	status := record.StatusEntry()
	points, ok := status["points"].([][]float64)
	if !ok {
		t.Fatalf("status points type = %T", status["points"])
	}
	if !reflect.DeepEqual(points, matrix) {
		t.Fatalf("status points = %#v, want %#v", points, matrix)
	}

	restored, err := RestoreZMeshFromProfile(NewStoredProfileRecord(status["points"], status["mesh_params"].(map[string]interface{})))
	if err != nil {
		t.Fatalf("RestoreZMeshFromProfile() error = %v", err)
	}
	if got := restored.Get_probed_matrix(); !reflect.DeepEqual(got, matrix) {
		t.Fatalf("restored matrix = %#v, want %#v", got, matrix)
	}

	matrix[0][0] = 99
	params["min_x"] = -99.0
	statusPoints := status["points"].([][]float64)
	statusParams := status["mesh_params"].(map[string]interface{})
	if statusPoints[0][0] != 0 {
		t.Fatalf("status entry reused point backing slice: %#v", statusPoints)
	}
	if statusParams["min_x"].(float64) != 0 {
		t.Fatalf("status entry reused mesh params: %#v", statusParams)
	}
	if record.MeshParams["min_x"].(float64) != 0 {
		t.Fatalf("record reused source mesh params: %#v", record.MeshParams)
	}
}

func TestCloneProfileStoreDetached(t *testing.T) {
	record := NewProfileRecord([][]float64{{1, 2}, {3, 4}}, testMeshParams())
	profiles := map[string]interface{}{"default": record.StatusEntry()}
	cloned := CloneProfileStore(profiles)

	profiles["default"].(map[string]interface{})["points"].([][]float64)[0][0] = 99
	profiles["default"].(map[string]interface{})["mesh_params"].(map[string]interface{})["min_x"] = -9.0

	entry := cloned["default"].(map[string]interface{})
	if got := entry["points"].([][]float64)[0][0]; got != 1 {
		t.Fatalf("cloned points reused backing slice: %v", got)
	}
	if got := entry["mesh_params"].(map[string]interface{})["min_x"].(float64); got != 0 {
		t.Fatalf("cloned params reused backing map: %v", got)
	}
}

func TestDecodeStoredProfileStoreSeparatesCompatibleProfiles(t *testing.T) {
	inputs := []StoredProfileInput{
		{
			Name:       "default",
			Version:    ProfileVersion,
			PointsData: [][]float64{{1, 2}, {3, 4}},
			MeshParams: testMeshParams(),
		},
		{
			Name:       "legacy",
			Version:    ProfileVersion - 1,
			PointsData: [][]float64{{9, 9}},
			MeshParams: testMeshParams(),
		},
	}
	store := DecodeStoredProfileStore(inputs)

	if got := len(store.Profiles); got != 1 {
		t.Fatalf("profiles len = %d, want 1", got)
	}
	if _, ok := store.Profiles["default"]; !ok {
		t.Fatalf("expected compatible profile to be loaded, got %#v", store.Profiles)
	}
	if got := len(store.Incompatible); got != 1 || store.Incompatible[0].Name != "legacy" || store.Incompatible[0].Version != ProfileVersion-1 {
		t.Fatalf("incompatible = %#v, want [{legacy %d}]", store.Incompatible, ProfileVersion-1)
	}

	inputs[0].PointsData.([][]float64)[0][0] = 99
	if got := store.Profiles["default"].(map[string]interface{})["points"].([][]float64)[0][0]; got != 1 {
		t.Fatalf("decoded profile should clone input points, got %v", got)
	}
}

func TestProfileManagerSaveLoadRemove(t *testing.T) {
	manager := NewProfileManager([]StoredProfileInput{{
		Name:       "existing",
		Version:    ProfileVersion,
		PointsData: [][]float64{{1, 2}, {3, 4}},
		MeshParams: testMeshParams(),
	}})

	if _, err := manager.SaveProfile("missing", nil); !errors.Is(err, ErrProfileSaveWithoutMesh) {
		t.Fatalf("SaveProfile(nil) error = %v, want %v", err, ErrProfileSaveWithoutMesh)
	}

	mesh, err := RestoreZMeshFromProfile(NewProfileRecord([][]float64{{0, 1}, {2, 3}}, testMeshParams()))
	if err != nil {
		t.Fatalf("RestoreZMeshFromProfile() error = %v", err)
	}
	record, err := manager.SaveProfile("saved", mesh)
	if err != nil {
		t.Fatalf("SaveProfile() error = %v", err)
	}
	if got := manager.CurrentProfile(); got != "saved" {
		t.Fatalf("CurrentProfile() = %q, want saved", got)
	}
	if got := record.ConfigValues()["version"]; got != "1" {
		t.Fatalf("saved record version = %q, want 1", got)
	}

	restored, err := manager.LoadProfile("existing")
	if err != nil {
		t.Fatalf("LoadProfile(existing) error = %v", err)
	}
	if got := restored.Get_probed_matrix(); !reflect.DeepEqual(got, [][]float64{{1, 2}, {3, 4}}) {
		t.Fatalf("loaded matrix = %#v", got)
	}
	if got := manager.CurrentProfile(); got != "existing" {
		t.Fatalf("CurrentProfile() after load = %q, want existing", got)
	}

	if _, err := manager.LoadProfile("missing"); !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("LoadProfile(missing) error = %v, want %v", err, ErrUnknownProfile)
	}

	if removed := manager.RemoveProfile("existing"); !removed {
		t.Fatalf("RemoveProfile(existing) = false, want true")
	}
	if removed := manager.RemoveProfile("missing"); removed {
		t.Fatalf("RemoveProfile(missing) = true, want false")
	}
	if _, ok := manager.Profiles()["existing"]; ok {
		t.Fatalf("existing profile still present after removal")
	}

	if _, err := manager.SaveProfile("current", mesh); err != nil {
		t.Fatalf("SaveProfile(current) error = %v", err)
	}
	if removed := manager.RemoveProfile("current"); !removed {
		t.Fatalf("RemoveProfile(current) = false, want true")
	}
	if got := manager.CurrentProfile(); got != "" {
		t.Fatalf("CurrentProfile() after removing current = %q, want empty", got)
	}
	status := manager.BuildStatus(mesh).AsMap()
	if got := status["profiles"].(map[string]interface{}); len(got) != 1 {
		t.Fatalf("profiles len after removals = %d, want 1", len(got))
	}
}

func TestBuildMeshStatusAndMarshalMeshMap(t *testing.T) {
	mesh, err := RestoreZMeshFromProfile(NewProfileRecord([][]float64{{0, 1}, {2, 3}}, testMeshParams()))
	if err != nil {
		t.Fatalf("RestoreZMeshFromProfile() error = %v", err)
	}
	profiles := map[string]interface{}{"adaptive": NewProfileRecord([][]float64{{4, 5}, {6, 7}}, testMeshParams()).StatusEntry()}
	status := BuildMeshStatus("adaptive", profiles, mesh)
	statusMap := status.AsMap()

	if got := statusMap["profile_name"].(string); got != "adaptive" {
		t.Fatalf("profile_name = %q, want adaptive", got)
	}
	if got := statusMap["mesh_min"].([]float64); !reflect.DeepEqual(got, []float64{0, 0}) {
		t.Fatalf("mesh_min = %#v", got)
	}
	if got := statusMap["mesh_max"].([]float64); !reflect.DeepEqual(got, []float64{10, 10}) {
		t.Fatalf("mesh_max = %#v", got)
	}

	payload, err := MarshalMeshMap(mesh)
	if err != nil {
		t.Fatalf("MarshalMeshMap() error = %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := decoded["mesh_min"].([]interface{}); len(got) != 2 || got[0].(float64) != 0 || got[1].(float64) != 0 {
		t.Fatalf("decoded mesh_min = %#v", got)
	}
	if got := decoded["mesh_max"].([]interface{}); len(got) != 2 || got[0].(float64) != 10 || got[1].(float64) != 10 {
		t.Fatalf("decoded mesh_max = %#v", got)
	}
	if got := decoded["z_positions"].([]interface{}); len(got) != 2 {
		t.Fatalf("decoded z_positions rows = %d, want 2", len(got))
	}
}
