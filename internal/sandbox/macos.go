//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// macOSSandbox implements sandboxing using macOS sandbox-exec (Seatbelt).
type macOSSandbox struct {
	sandboxExecPath string
	policy          *Policy
	debug           bool
}

func newPlatformSandbox(kind string) Sandbox {
	switch kind {
	case "none", "off":
		return &noopSandbox{}
	default: // "auto", "seatbelt", "macos"
		if path, err := exec.LookPath("sandbox-exec"); err == nil {
			return &macOSSandbox{sandboxExecPath: path}
		}
		return &noopSandbox{}
	}
}

func (s *macOSSandbox) Name() string      { return "macos-seatbelt" }
func (s *macOSSandbox) IsAvailable() bool { return s.sandboxExecPath != "" }

func (s *macOSSandbox) Capabilities() Capabilities {
	return Capabilities{
		SupportsNamespaces:       false,
		SupportsSeccomp:          false,
		SupportsCgroups:          false,
		SupportsNetworkIsolation: true,
		SupportsFSIsolation:      true,
		SupportsResourceLimits:   false, // macOS uses rlimits separately
		RequiresRoot:             false,
		Description:              "macOS sandbox using Seatbelt (sandbox-exec)",
	}
}

func (s *macOSSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	return s.WrapWithPolicy(context.Background(), name, args, StandardPolicy())
}

func (s *macOSSandbox) WrapWithPolicy(_ context.Context, name string, args []string, policy *Policy) (string, []string) {
	if policy == nil {
		policy = StandardPolicy()
	}

	profile := s.generateSeatbeltProfile(policy)
	newArgs := append([]string{"-p", profile, name}, args...)
	return s.sandboxExecPath, newArgs
}

// generateSeatbeltProfile generates a Seatbelt profile from a policy.
func (s *macOSSandbox) generateSeatbeltProfile(policy *Policy) string {
	var sb strings.Builder

	// Version declaration
	sb.WriteString("(version 1)\n")

	// Default action based on policy strictness
	if policy.Name == "strict" {
		sb.WriteString("(deny default)\n")
	} else {
		sb.WriteString("(allow default)\n")
	}

	// File system restrictions
	s.addFileSystemRules(&sb, policy)

	// Network restrictions
	s.addNetworkRules(&sb, policy)

	// Process restrictions
	s.addProcessRules(&sb, policy)

	// System restrictions
	s.addSystemRules(&sb, policy)

	return sb.String()
}

func (s *macOSSandbox) addFileSystemRules(sb *strings.Builder, policy *Policy) {
	// Deny write to root filesystem by default for non-permissive policies
	if policy.Name != "permissive" && policy.Name != "development" {
		sb.WriteString("(deny file-write* (subpath \"/\"))\n")
	}

	// Allow writes to temp directory
	sb.WriteString("(allow file-write* (subpath \"/tmp\"))\n")
	sb.WriteString("(allow file-write* (subpath \"/private/tmp\"))\n")
	sb.WriteString("(allow file-write* (subpath \"/var/folders\"))\n")
	sb.WriteString("(allow file-write* (subpath \"/private/var/folders\"))\n")

	// Allow writes to home directory temp
	sb.WriteString("(allow file-write* (subpath (param \"TMPDIR\")))\n")

	// Allow writes to specified paths
	for _, path := range policy.FileSystem.ReadWritePaths {
		sb.WriteString(fmt.Sprintf("(allow file-write* (subpath \"%s\"))\n", path))
		sb.WriteString(fmt.Sprintf("(allow file-read* (subpath \"%s\"))\n", path))
	}

	// Allow reads from specified paths
	for _, path := range policy.FileSystem.ReadOnlyPaths {
		sb.WriteString(fmt.Sprintf("(allow file-read* (subpath \"%s\"))\n", path))
	}

	// Deny access to specified paths
	for _, path := range policy.FileSystem.DeniedPaths {
		sb.WriteString(fmt.Sprintf("(deny file-read* (subpath \"%s\"))\n", path))
		sb.WriteString(fmt.Sprintf("(deny file-write* (subpath \"%s\"))\n", path))
	}

	// Working directory access
	if policy.FileSystem.WorkDir != "" {
		sb.WriteString(fmt.Sprintf("(allow file-write* (subpath \"%s\"))\n", policy.FileSystem.WorkDir))
		sb.WriteString(fmt.Sprintf("(allow file-read* (subpath \"%s\"))\n", policy.FileSystem.WorkDir))
	}

	// Always allow reading standard system paths
	systemReadPaths := []string{
		"/usr",
		"/bin",
		"/sbin",
		"/System",
		"/Library/Frameworks",
		"/Applications/Xcode.app/Contents/Developer",
	}
	for _, path := range systemReadPaths {
		sb.WriteString(fmt.Sprintf("(allow file-read* (subpath \"%s\"))\n", path))
	}

	// Allow reading user home for certain configs
	sb.WriteString("(allow file-read* (home-subpath \".config\"))\n")
	sb.WriteString("(allow file-read* (home-subpath \".local\"))\n")

	// Device access
	if !policy.FileSystem.AllowDevices {
		sb.WriteString("(deny file-read* (subpath \"/dev\"))\n")
		// Allow essential devices
		sb.WriteString("(allow file-read* (literal \"/dev/null\"))\n")
		sb.WriteString("(allow file-read* (literal \"/dev/zero\"))\n")
		sb.WriteString("(allow file-read* (literal \"/dev/random\"))\n")
		sb.WriteString("(allow file-read* (literal \"/dev/urandom\"))\n")
		sb.WriteString("(allow file-write* (literal \"/dev/null\"))\n")
	}
}

func (s *macOSSandbox) addNetworkRules(sb *strings.Builder, policy *Policy) {
	if !policy.Network.Enabled {
		sb.WriteString("(deny network*)\n")

		// Allow loopback if specified
		if policy.Network.AllowLoopback {
			sb.WriteString("(allow network* (local ip \"localhost:*\"))\n")
			sb.WriteString("(allow network* (local ip \"127.0.0.1:*\"))\n")
			sb.WriteString("(allow network* (remote ip \"localhost:*\"))\n")
			sb.WriteString("(allow network* (remote ip \"127.0.0.1:*\"))\n")
		}

		// Allow DNS if specified
		if policy.Network.AllowDNS {
			sb.WriteString("(allow network-outbound (remote unix-socket (path-literal \"/var/run/mDNSResponder\")))\n")
			sb.WriteString("(allow network-outbound (remote ip \"*:53\"))\n")
		}
	} else {
		// Network enabled, apply restrictions
		sb.WriteString("(allow network*)\n")

		// Restrict to allowed hosts if specified
		if len(policy.Network.AllowedHosts) > 0 {
			sb.WriteString("(deny network-outbound)\n")
			for _, host := range policy.Network.AllowedHosts {
				sb.WriteString(fmt.Sprintf("(allow network-outbound (remote ip \"%s:*\"))\n", host))
			}
			// Always allow loopback when network is enabled
			sb.WriteString("(allow network-outbound (remote ip \"localhost:*\"))\n")
			sb.WriteString("(allow network-outbound (remote ip \"127.0.0.1:*\"))\n")
		}

		// Restrict to allowed ports if specified
		if len(policy.Network.AllowedPorts) > 0 {
			sb.WriteString("(deny network-outbound)\n")
			for _, port := range policy.Network.AllowedPorts {
				sb.WriteString(fmt.Sprintf("(allow network-outbound (remote ip \"*:%d\"))\n", port))
			}
		}
	}
}

func (s *macOSSandbox) addProcessRules(sb *strings.Builder, policy *Policy) {
	// Process execution
	if !policy.Process.AllowExec {
		sb.WriteString("(deny process-exec)\n")
		// Allow the sandbox-exec itself
		sb.WriteString("(allow process-exec (literal \"/usr/bin/sandbox-exec\"))\n")
	} else if len(policy.Process.AllowedExecutables) > 0 {
		// Only allow specific executables
		sb.WriteString("(deny process-exec)\n")
		for _, exe := range policy.Process.AllowedExecutables {
			sb.WriteString(fmt.Sprintf("(allow process-exec (literal \"%s\"))\n", exe))
		}
	}

	// Fork/spawn restrictions
	if !policy.Process.AllowFork {
		sb.WriteString("(deny process-fork)\n")
	}

	// Ptrace/debugging
	if !policy.Process.AllowPtrace {
		sb.WriteString("(deny system-privilege)\n")
	}
}

func (s *macOSSandbox) addSystemRules(sb *strings.Builder, policy *Policy) {
	// Always deny dangerous operations
	sb.WriteString("(deny system-kext*)\n")
	sb.WriteString("(deny nvram*)\n")

	// Deny sysctl writes
	sb.WriteString("(deny sysctl-write)\n")

	// Allow reading sysctl
	sb.WriteString("(allow sysctl-read)\n")

	// IPC restrictions
	if policy.Name == "strict" {
		sb.WriteString("(deny ipc-posix*)\n")
		sb.WriteString("(deny ipc-sysv*)\n")
	}

	// Mach bootstrap
	sb.WriteString("(allow mach-lookup)\n")
	sb.WriteString("(allow mach-register)\n")

	// Allow signal sending to own process
	sb.WriteString("(allow signal (target self))\n")
}

// SeatbeltProfile contains a complete Seatbelt profile.
type SeatbeltProfile struct {
	Name        string
	Description string
	Content     string
}

// PredefinedProfiles returns a list of predefined Seatbelt profiles.
func PredefinedProfiles() []SeatbeltProfile {
	return []SeatbeltProfile{
		{
			Name:        "no-network",
			Description: "Denies all network access",
			Content: `(version 1)
(allow default)
(deny network*)`,
		},
		{
			Name:        "no-write",
			Description: "Denies all file writes except tmp",
			Content: `(version 1)
(allow default)
(deny file-write* (subpath "/"))
(allow file-write* (subpath "/tmp"))
(allow file-write* (subpath "/private/tmp"))
(allow file-write* (subpath (param "TMPDIR")))`,
		},
		{
			Name:        "pure-computation",
			Description: "Minimal access for pure computation",
			Content: `(version 1)
(deny default)
(allow process-exec)
(allow file-read* (subpath "/usr"))
(allow file-read* (subpath "/bin"))
(allow file-read* (subpath "/System"))
(allow file-read* (literal "/dev/null"))
(allow file-read* (literal "/dev/urandom"))
(allow file-write* (literal "/dev/null"))
(allow mach-lookup)
(allow signal (target self))
(allow sysctl-read)`,
		},
		{
			Name:        "development",
			Description: "Development-friendly restrictions",
			Content: `(version 1)
(allow default)
(deny network-outbound (remote ip "*:22"))
(deny system-kext*)
(deny nvram*)
(deny sysctl-write)`,
		},
	}
}

// MacOSResourceLimits sets resource limits using rlimit.
type MacOSResourceLimits struct {
	CPUTime      int64 // Seconds
	FileSize     int64 // Bytes
	DataSize     int64 // Bytes
	StackSize    int64 // Bytes
	CoreSize     int64 // Bytes (0 = no core dumps)
	OpenFiles    int64 // Count
	MaxProcesses int64 // Count
}

// ApplyResourceLimits applies resource limits to the current process.
// This should be called in the child process before exec.
func (r *MacOSResourceLimits) ApplyResourceLimits() error {
	// Note: Implementation would use syscall.Setrlimit
	// This is a placeholder for the resource limit application
	return nil
}
