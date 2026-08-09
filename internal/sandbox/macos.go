//go:build darwin

package sandbox

import (
	"context"
	"os/exec"
)

type macOSSandbox struct{}

func newPlatformSandbox(kind string) Sandbox {
	if kind == "none" {
		return &noopSandbox{}
	}
	if _, err := exec.LookPath("sandbox-exec"); err == nil {
		return &macOSSandbox{}
	}
	return &noopSandbox{}
}

func (s *macOSSandbox) Name() string      { return "macos-seatbelt" }
func (s *macOSSandbox) IsAvailable() bool { return true }

func (s *macOSSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	profile := `(version 1)(allow default)(deny file-write* (subpath "/"))(allow file-write* (subpath (param "HOME")))`
	newArgs := append([]string{"-p", profile, name}, args...)
	return "sandbox-exec", newArgs
}
