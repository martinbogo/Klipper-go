//go:build !linux

package serialhdl

// setTIOCEXCL is a no-op on non-Linux platforms.
func setTIOCEXCL(fd uintptr) error {
	return nil
}
