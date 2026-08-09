package sandbox

import "context"

// noopSandbox is a no-op fallback that does not apply any restrictions.
type noopSandbox struct{}

func (s *noopSandbox) Name() string      { return "noop" }
func (s *noopSandbox) IsAvailable() bool { return false }

func (s *noopSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	return name, args
}

type Sandbox interface {
	// Wrap wraps a command with sandbox restrictions.
	Wrap(ctx context.Context, name string, args []string) (string, []string)
	// IsAvailable reports whether the sandbox mechanism is available.
	IsAvailable() bool
	// Name returns the sandbox implementation name.
	Name() string
}

// New returns the appropriate sandbox for the current OS and config.
func New(kind string) Sandbox {
	return newPlatformSandbox(kind)
}
