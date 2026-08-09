//go:build windows

package sandbox

import "context"

type windowsSandbox struct{}

func newPlatformSandbox(kind string) Sandbox {
	return &noopSandbox{}
}

func (s *windowsSandbox) Name() string      { return "windows-appcontainer" }
func (s *windowsSandbox) IsAvailable() bool { return false }

func (s *windowsSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	return name, args
}
