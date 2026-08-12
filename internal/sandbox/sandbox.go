package sandbox

import (
	"context"
	"fmt"
	"runtime"
)

// Sandbox defines the interface for OS-level sandboxing.
type Sandbox interface {
	// Wrap wraps a command with sandbox restrictions.
	Wrap(ctx context.Context, name string, args []string) (string, []string)

	// WrapWithPolicy wraps a command using the specified security policy.
	WrapWithPolicy(ctx context.Context, name string, args []string, policy *Policy) (string, []string)

	// IsAvailable reports whether the sandbox mechanism is available.
	IsAvailable() bool

	// Name returns the sandbox implementation name.
	Name() string

	// Capabilities returns the capabilities of this sandbox implementation.
	Capabilities() Capabilities
}

// Capabilities describes what the sandbox implementation can do.
type Capabilities struct {
	// SupportsNamespaces indicates support for namespace isolation.
	SupportsNamespaces bool

	// SupportsSeccomp indicates support for system call filtering.
	SupportsSeccomp bool

	// SupportsCgroups indicates support for resource limits via cgroups.
	SupportsCgroups bool

	// SupportsNetworkIsolation indicates support for network isolation.
	SupportsNetworkIsolation bool

	// SupportsFSIsolation indicates support for filesystem isolation.
	SupportsFSIsolation bool

	// SupportsResourceLimits indicates support for resource limiting.
	SupportsResourceLimits bool

	// RequiresRoot indicates if root privileges are needed.
	RequiresRoot bool

	// Description provides a human-readable description.
	Description string
}

// SandboxConfig configures sandbox creation.
type SandboxConfig struct {
	// Kind specifies the sandbox type: "auto", "bubblewrap", "namespaces",
	// "seatbelt", "appcontainer", or "none".
	Kind string

	// Policy is the security policy to apply.
	Policy *Policy

	// Debug enables debug output.
	Debug bool

	// AllowNetwork overrides policy to allow network.
	AllowNetwork bool

	// WorkDir specifies the working directory.
	WorkDir string
}

// New returns the appropriate Sandbox for the current OS and configuration.
// The kind parameter selects the sandbox type: "auto", "macos", "docker",
// "namespaces", or "off". When kind is "auto" or empty the platform default is
// chosen automatically (sandbox-exec on macOS, bubblewrap on Linux).
func New(kind string) Sandbox {
	return newPlatformSandbox(kind)
}

// NewWithConfig creates a sandbox with the specified configuration.
func NewWithConfig(config *SandboxConfig) Sandbox {
	if config == nil {
		config = &SandboxConfig{Kind: "auto"}
	}
	return newPlatformSandbox(config.Kind)
}

// Available returns all available sandbox implementations for the current OS.
func Available() []string {
	var available []string

	// Check platform-specific sandboxes
	switch runtime.GOOS {
	case "linux":
		s := newPlatformSandbox("auto")
		if s.IsAvailable() {
			available = append(available, s.Name())
		}
	case "darwin":
		s := newPlatformSandbox("auto")
		if s.IsAvailable() {
			available = append(available, s.Name())
		}
	case "windows":
		// Windows sandboxing support is limited
		available = append(available, "noop")
	}

	if len(available) == 0 {
		available = append(available, "noop")
	}

	return available
}

// Describe returns information about a sandbox type.
func Describe(kind string) (string, error) {
	s := newPlatformSandbox(kind)
	if s == nil {
		return "", fmt.Errorf("unknown sandbox type: %s", kind)
	}

	caps := s.Capabilities()
	return fmt.Sprintf("%s: %s (available: %v)", s.Name(), caps.Description, s.IsAvailable()), nil
}

// noopSandbox is a no-op fallback that does not apply any restrictions.
type noopSandbox struct{}

func (s *noopSandbox) Name() string      { return "noop" }
func (s *noopSandbox) IsAvailable() bool { return false }

func (s *noopSandbox) Capabilities() Capabilities {
	return Capabilities{
		Description: "No sandboxing - commands run without restrictions",
	}
}

func (s *noopSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	return name, args
}

func (s *noopSandbox) WrapWithPolicy(_ context.Context, name string, args []string, _ *Policy) (string, []string) {
	return name, args
}
