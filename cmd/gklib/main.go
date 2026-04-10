package main

import (
	"goklipper/common/logger"
	"goklipper/common/utils/sys"
	"goklipper/project"
	"time"
)

func main() {
	logger.Debugf("main thread %d running", sys.GetGID())
	project.ModuleKlipper()
	klipper := project.NewKlipper()
	klipper.Main()
	for {
		time.Sleep(1 * time.Second)
	}
}
