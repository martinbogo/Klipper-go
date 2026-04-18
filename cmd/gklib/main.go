package main

import (
	"goklipper/common/logger"
	pprofutil "goklipper/common/pprof"
	"goklipper/common/utils/sys"
	"goklipper/project"
	"os"
	"strings"
	"time"
)

func maybeStartPprof() {
	addr := strings.TrimSpace(os.Getenv("GKLIB_PPROF_ADDR"))
	if addr == "" {
		return
	}
	prefix := strings.TrimSpace(os.Getenv("GKLIB_PPROF_PREFIX"))
	logger.Infof("Starting pprof endpoint at %s (prefix=%q)", addr, prefix)
	pprofutil.Run(addr, prefix)
}

func main() {
	logger.Debugf("main thread %d running", sys.GetGID())
	maybeStartPprof()
	project.ModuleKlipper()
	klipper := project.NewKlipper()
	klipper.Main()
	for {
		time.Sleep(1 * time.Second)
	}
}
