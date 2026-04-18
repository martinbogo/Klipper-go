package moduleinit

// WrapModuleInit adapts a typed module init function to the generic module
// registry callback signature used by printer.ModuleRegistry.
func WrapModuleInit[T any](init func(T) interface{}) func(interface{}) interface{} {
	return func(section interface{}) interface{} {
		return init(section.(T))
	}
}
