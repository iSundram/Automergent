package sandbox

import "context"

// Sandbox defines the interface for OS-level sandboxing.
type Sandbox interface {
	// Wrap wraps a command with sandbox restrictions.
	Wrap(ctx context.Context, name string, args []string) (string, []string)
	// IsAvailable reports whether the sandbox mechanism is available.
	IsAvailable() bool
	// Name returns the sandbox implementation name.
	Name() string
}

// New returns the appropriate Sandbox for the current OS and configuration.
// The kind parameter selects the sandbox type: "auto", "macos", "docker",
// "namespaces", or "off". When kind is "auto" or empty the platform default is
// chosen automatically (sandbox-exec on macOS, bubblewrap on Linux).
func New(kind string) Sandbox {
	return newPlatformSandbox(kind)
}

// noopSandbox is a no-op fallback that does not apply any restrictions.
type noopSandbox struct{}

func (s *noopSandbox) Name() string      { return "noop" }
func (s *noopSandbox) IsAvailable() bool { return false }

func (s *noopSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	return name, args
}
