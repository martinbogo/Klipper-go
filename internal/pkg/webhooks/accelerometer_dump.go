package webhooks

import reportpkg "goklipper/internal/pkg/motion/report"

type AccelerometerDumpRegistry interface {
	RegisterMuxEndpoint(path, key, value string, callback func(RuntimeRequest))
}

type AccelerometerDumpModule interface {
	AddClient(reportpkg.APIDumpClient, map[string]interface{})
	Get_name() string
}

func RegisterAccelerometerDumpEndpoint(registry AccelerometerDumpRegistry, path string, module AccelerometerDumpModule) {
	RegisterTypedMuxEndpoint[*ConnectedRequest](registry, path, "sensor", module.Get_name(), func(request *ConnectedRequest) {
		module.AddClient(NewAPIDumpClient(request.Connection()), request.Get_dict("response_template", nil))
		request.Send(map[string][]string{"header": {"time", "x_acceleration", "y_acceleration", "z_acceleration"}})
	})
}
