//go:build linux

package sandbox

import (
	"context"
	"os/exec"
)

type linuxSandbox struct{}

func newPlatformSandbox(kind string) Sandbox {
	if kind == "none" {
		return &noopSandbox{}
	}
	if _, err := exec.LookPath("bwrap"); err == nil {
		return &linuxSandbox{}
	}
	return &noopSandbox{}
}

func (s *linuxSandbox) Name() string      { return "linux-bubblewrap" }
func (s *linuxSandbox) IsAvailable() bool { return true }

func (s *linuxSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	bwrapArgs := []string{
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/lib", "/lib",
		"--ro-bind", "/lib64", "/lib64",
		"--proc", "/proc",
		"--dev", "/dev",
		"--bind", "/tmp", "/tmp",
		"--unshare-net",
		"--",
		name,
	}
	bwrapArgs = append(bwrapArgs, args...)
	return "bwrap", bwrapArgs
}
