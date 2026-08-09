//go:build !darwin && !linux && !windows

package sandbox

func newPlatformSandbox(_ string) Sandbox { return &noopSandbox{} }
