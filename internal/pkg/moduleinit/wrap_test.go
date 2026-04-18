package moduleinit

import "testing"

func TestWrapModuleInitPreservesTypedInput(t *testing.T) {
	type sampleConfig struct {
		Name string
	}

	wrapped := WrapModuleInit(func(cfg *sampleConfig) interface{} {
		return cfg.Name
	})

	if got := wrapped(&sampleConfig{Name: "ok"}); got != "ok" {
		t.Fatalf("expected wrapped init to return typed value, got %#v", got)
	}
}
