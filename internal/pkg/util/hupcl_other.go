//go:build !linux

package util

func clearHUPCLImpl(fd uintptr) {
	_ = fd
}
