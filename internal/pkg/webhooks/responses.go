package webhooks

import (
	"os"
	"path/filepath"
	"runtime"
)

// BuildInfoResponse constructs the response map for the "info" webhook endpoint.
func BuildInfoResponse(state, stateMessage string, startArgs map[string]interface{}) map[string]interface{} {
	_, filename, _, _ := runtime.Caller(1)
	srcPath := filepath.Dir(filename)
	klipperPath := filepath.Clean(filepath.Join(srcPath, ".."))

	response := map[string]interface{}{
		"state":         state,
		"state_message": stateMessage,
		"hostname":      "",
		"klipper_path":  klipperPath,
		"python_path":   "",
		"process_id":    os.Getpid(),
		"user_id":       os.Getuid(),
		"group_id":      os.Getgid(),
	}
	for _, sa := range []string{"log_file", "config_file", "software_version", "cpu_info"} {
		saValue := startArgs[sa]
		if saValue != nil {
			response[sa] = saValue.(string)
		}
	}
	return response
}

// BuildFilamentHubInfoResponse constructs the static response map for
// the "filament_hub/info" webhook endpoint.
func BuildFilamentHubInfoResponse() map[string]interface{} {
	return map[string]interface{}{
		"infos": []interface{}{
			map[string]interface{}{
				"id":                0,
				"slots":             4,
				"SN":                "",
				"date":              "",
				"model":             "Anycubic Color Engine Pro",
				"firmware":          "V1.3.863",
				"structure_version": "0",
			},
		},
	}
}
