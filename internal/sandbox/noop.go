//go:build !darwin && !linux && !windows

package sandbox

import "context"

func newPlatformSandbox(_ string) Sandbox { return &noopSandbox{} }

// Capabilities returns capabilities for unsupported platforms.
func (s *noopSandbox) WrapWithPolicy(_ context.Context, name string, args []string, _ *Policy) (string, []string) {
	return name, args
}
