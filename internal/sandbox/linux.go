//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// linuxSandbox implements sandboxing using bubblewrap and Linux namespaces.
type linuxSandbox struct {
	bwrapPath    string
	seccompAvail bool
	cgroupsAvail bool
	policy       *Policy
	debug        bool
}

// linuxNamespaceSandbox implements sandboxing using raw Linux namespaces.
type linuxNamespaceSandbox struct {
	policy *Policy
	debug  bool
}

func newPlatformSandbox(kind string) Sandbox {
	switch kind {
	case "none", "off":
		return &noopSandbox{}
	case "namespaces":
		return &linuxNamespaceSandbox{}
	case "bubblewrap", "bwrap":
		if path, err := exec.LookPath("bwrap"); err == nil {
			return &linuxSandbox{
				bwrapPath:    path,
				seccompAvail: checkSeccompAvailable(),
				cgroupsAvail: checkCgroupsAvailable(),
			}
		}
		return &noopSandbox{}
	default: // "auto" or empty
		// Prefer bubblewrap if available
		if path, err := exec.LookPath("bwrap"); err == nil {
			return &linuxSandbox{
				bwrapPath:    path,
				seccompAvail: checkSeccompAvailable(),
				cgroupsAvail: checkCgroupsAvailable(),
			}
		}
		// Fall back to namespace sandbox if we have capabilities
		if os.Getuid() == 0 || checkUserNamespacesAvailable() {
			return &linuxNamespaceSandbox{}
		}
		return &noopSandbox{}
	}
}

// checkSeccompAvailable checks if seccomp is available.
func checkSeccompAvailable() bool {
	_, err := os.Stat("/proc/sys/kernel/seccomp")
	return err == nil
}

// checkCgroupsAvailable checks if cgroups v2 is available.
func checkCgroupsAvailable() bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "cgroup2")
}

// checkUserNamespacesAvailable checks if unprivileged user namespaces are available.
func checkUserNamespacesAvailable() bool {
	data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone")
	if err != nil {
		// File doesn't exist, check if we can create namespaces anyway
		_, err = os.ReadFile("/proc/self/ns/user")
		return err == nil
	}
	return strings.TrimSpace(string(data)) == "1"
}

// ============================================================================
// Bubblewrap Sandbox Implementation
// ============================================================================

func (s *linuxSandbox) Name() string { return "linux-bubblewrap" }

func (s *linuxSandbox) IsAvailable() bool {
	return s.bwrapPath != ""
}

func (s *linuxSandbox) Capabilities() Capabilities {
	return Capabilities{
		SupportsNamespaces:       true,
		SupportsSeccomp:          s.seccompAvail,
		SupportsCgroups:          s.cgroupsAvail,
		SupportsNetworkIsolation: true,
		SupportsFSIsolation:      true,
		SupportsResourceLimits:   s.cgroupsAvail,
		RequiresRoot:             false,
		Description:              "Linux sandbox using bubblewrap with namespace isolation",
	}
}

func (s *linuxSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	// Use default standard policy
	return s.WrapWithPolicy(context.Background(), name, args, StandardPolicy())
}

func (s *linuxSandbox) WrapWithPolicy(_ context.Context, name string, args []string, policy *Policy) (string, []string) {
	if policy == nil {
		policy = StandardPolicy()
	}

	bwrapArgs := s.buildBwrapArgs(policy, name)
	bwrapArgs = append(bwrapArgs, args...)
	return s.bwrapPath, bwrapArgs
}

func (s *linuxSandbox) buildBwrapArgs(policy *Policy, command string) []string {
	args := []string{}

	// Die with parent process
	args = append(args, "--die-with-parent")

	// Create new session
	if policy.Process.NewSession {
		args = append(args, "--new-session")
	}

	// Unshare namespaces
	args = append(args, "--unshare-user")
	args = append(args, "--unshare-pid")
	args = append(args, "--unshare-uts")
	args = append(args, "--unshare-ipc")
	args = append(args, "--unshare-cgroup")

	// Network isolation
	if !policy.Network.Enabled {
		args = append(args, "--unshare-net")
	}

	// Set up filesystem
	args = s.setupFilesystem(args, policy)

	// Set up proc and dev
	if policy.FileSystem.AllowProc {
		args = append(args, "--proc", "/proc")
	}

	if policy.FileSystem.AllowDevices {
		args = append(args, "--dev", "/dev")
	} else {
		args = append(args, "--dev-bind", "/dev/null", "/dev/null")
		args = append(args, "--dev-bind", "/dev/zero", "/dev/zero")
		args = append(args, "--dev-bind", "/dev/urandom", "/dev/urandom")
	}

	// Set hostname
	args = append(args, "--hostname", "sandbox")

	// Clear environment for strict policies
	if policy.Name == "strict" {
		args = append(args, "--clearenv")
		args = append(args, "--setenv", "PATH", "/usr/bin:/bin")
		args = append(args, "--setenv", "HOME", "/tmp")
	}

	// Add seccomp filter if available
	// NOTE: Custom seccomp filtering is not yet implemented (see generateSeccompProfile).
	// Bubblewrap applies its own default seccomp profile which provides baseline protection.
	if s.seccompAvail && len(policy.Syscalls.DeniedSyscalls) > 0 {
		seccompPath := s.generateSeccompProfile(policy)
		if seccompPath != "" {
			args = append(args, "--seccomp", "3")
			// Note: bubblewrap expects seccomp filter on fd 3
		}
	}

	// Drop capabilities
	for _, cap := range policy.Process.DropCapabilities {
		if cap == "ALL" {
			args = append(args, "--cap-drop", "ALL")
			break
		}
		args = append(args, "--cap-drop", cap)
	}

	// Command separator
	args = append(args, "--")

	// Command to run
	args = append(args, command)

	return args
}

func (s *linuxSandbox) setupFilesystem(args []string, policy *Policy) []string {
	// Create tmpfs for tmp
	args = append(args, "--tmpfs", "/tmp")

	// Read-only bindings for system directories
	systemDirs := []string{"/usr", "/lib", "/lib64", "/bin", "/sbin"}
	for _, dir := range systemDirs {
		if _, err := os.Stat(dir); err == nil {
			args = append(args, "--ro-bind", dir, dir)
		}
	}

	// Add explicit read-only paths from policy
	for _, path := range policy.FileSystem.ReadOnlyPaths {
		if _, err := os.Stat(path); err == nil {
			args = append(args, "--ro-bind", path, path)
		}
	}

	// Add read-write paths from policy
	for _, path := range policy.FileSystem.ReadWritePaths {
		if _, err := os.Stat(path); err == nil {
			args = append(args, "--bind", path, path)
		}
	}

	// Working directory
	if policy.FileSystem.WorkDir != "" {
		if _, err := os.Stat(policy.FileSystem.WorkDir); err == nil {
			args = append(args, "--bind", policy.FileSystem.WorkDir, policy.FileSystem.WorkDir)
			args = append(args, "--chdir", policy.FileSystem.WorkDir)
		}
	}

	// Temp directory
	if policy.FileSystem.TempDir != "" {
		args = append(args, "--bind", policy.FileSystem.TempDir, "/tmp")
	}

	// Symlinks for common paths
	args = append(args, "--symlink", "/usr/lib64", "/lib64")

	return args
}

func (s *linuxSandbox) generateSeccompProfile(policy *Policy) string {
	// Seccomp BPF generation is intentionally not implemented here.
	//
	// SECURITY NOTE:
	// - Kernel support for seccomp (presence of /proc/sys/kernel/seccomp) does not imply
	//   that this package can generate or apply custom seccomp BPF programs.
	// - Generating and loading BPF requires libseccomp or equivalent (e.g. github.com/seccomp/libseccomp-golang),
	//   which is not linked into this module by default. Implementers should add libseccomp
	//   integration if they need custom syscall whitelisting.
	// - When this returns an empty string, no custom seccomp filter will be passed to
	//   bubblewrap; bubblewrap's internal/default profile (if any) will be relied upon.
	//
	// If strict syscall filtering is required, extend this function to generate BPF and
	// return an appropriate descriptor/path that bubblewrap can consume.
	return ""
}

// ============================================================================
// Namespace Sandbox Implementation (without bubblewrap)
// ============================================================================

func (s *linuxNamespaceSandbox) Name() string { return "linux-namespaces" }

func (s *linuxNamespaceSandbox) IsAvailable() bool {
	return os.Getuid() == 0 || checkUserNamespacesAvailable()
}

func (s *linuxNamespaceSandbox) Capabilities() Capabilities {
	return Capabilities{
		SupportsNamespaces:       true,
		SupportsSeccomp:          false,
		SupportsCgroups:          false,
		SupportsNetworkIsolation: true,
		SupportsFSIsolation:      true,
		SupportsResourceLimits:   false,
		RequiresRoot:             os.Getuid() != 0 && !checkUserNamespacesAvailable(),
		Description:              "Linux sandbox using raw namespace syscalls",
	}
}

func (s *linuxNamespaceSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	return s.WrapWithPolicy(context.Background(), name, args, StandardPolicy())
}

func (s *linuxNamespaceSandbox) WrapWithPolicy(_ context.Context, name string, args []string, policy *Policy) (string, []string) {
	if policy == nil {
		policy = StandardPolicy()
	}

	// Use unshare command for namespace isolation
	unshareArgs := []string{}

	// Map root user in new namespace
	unshareArgs = append(unshareArgs, "--map-root-user")

	// Create new namespaces
	unshareArgs = append(unshareArgs, "--user")
	unshareArgs = append(unshareArgs, "--pid")
	unshareArgs = append(unshareArgs, "--fork")
	unshareArgs = append(unshareArgs, "--mount-proc")

	// Network isolation
	if !policy.Network.Enabled {
		unshareArgs = append(unshareArgs, "--net")
	}

	// IPC isolation
	unshareArgs = append(unshareArgs, "--ipc")

	// UTS namespace (hostname)
	unshareArgs = append(unshareArgs, "--uts")

	// Add the actual command
	unshareArgs = append(unshareArgs, name)
	unshareArgs = append(unshareArgs, args...)

	return "unshare", unshareArgs
}

// ============================================================================
// Cgroup Controller for Resource Limits
// ============================================================================

// CgroupController manages cgroup-based resource limits.
type CgroupController struct {
	cgroupPath string
	name       string
}

// NewCgroupController creates a new cgroup controller.
func NewCgroupController(name string) (*CgroupController, error) {
	cgroupBase := "/sys/fs/cgroup"

	// Check for cgroup v2
	if _, err := os.Stat(filepath.Join(cgroupBase, "cgroup.controllers")); err != nil {
		return nil, fmt.Errorf("cgroup v2 not available")
	}

	cgroupPath := filepath.Join(cgroupBase, "automergent-sandbox", name)
	// Create cgroup directory with owner-only permissions to avoid information leakage.
	if err := os.MkdirAll(cgroupPath, 0700); err != nil {
		return nil, fmt.Errorf("creating cgroup: %w", err)
	}

	return &CgroupController{
		cgroupPath: cgroupPath,
		name:       name,
	}, nil
}

// SetMemoryLimit sets the memory limit in bytes.
func (c *CgroupController) SetMemoryLimit(bytes int64) error {
	return c.writeValue("memory.max", strconv.FormatInt(bytes, 10))
}

// SetMemorySwapLimit sets the memory+swap limit in bytes.
func (c *CgroupController) SetMemorySwapLimit(bytes int64) error {
	return c.writeValue("memory.swap.max", strconv.FormatInt(bytes, 10))
}

// SetCPUQuota sets CPU quota as percentage (100 = 100% of one CPU).
func (c *CgroupController) SetCPUQuota(percent int) error {
	// CPU quota is specified as microseconds per period (100000us = 100ms)
	period := 100000
	quota := percent * 1000 // Convert percentage to microseconds
	return c.writeValue("cpu.max", fmt.Sprintf("%d %d", quota, period))
}

// SetCPUShares sets the CPU shares for scheduling priority.
func (c *CgroupController) SetCPUShares(shares int) error {
	return c.writeValue("cpu.weight", strconv.Itoa(shares))
}

// SetIOWeight sets the I/O weight (1-1000).
func (c *CgroupController) SetIOWeight(weight int) error {
	if weight < 1 || weight > 1000 {
		return fmt.Errorf("IO weight must be between 1 and 1000")
	}
	return c.writeValue("io.weight", strconv.Itoa(weight))
}

// SetPIDLimit sets the maximum number of processes.
func (c *CgroupController) SetPIDLimit(max int) error {
	return c.writeValue("pids.max", strconv.Itoa(max))
}

// AddProcess adds a process to this cgroup.
func (c *CgroupController) AddProcess(pid int) error {
	return c.writeValue("cgroup.procs", strconv.Itoa(pid))
}

// GetMemoryUsage returns current memory usage in bytes.
func (c *CgroupController) GetMemoryUsage() (int64, error) {
	data, err := os.ReadFile(filepath.Join(c.cgroupPath, "memory.current"))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// GetCPUUsage returns CPU usage in microseconds.
func (c *CgroupController) GetCPUUsage() (int64, error) {
	data, err := os.ReadFile(filepath.Join(c.cgroupPath, "cpu.stat"))
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "usage_usec") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return strconv.ParseInt(parts[1], 10, 64)
			}
		}
	}

	return 0, fmt.Errorf("usage_usec not found")
}

// Cleanup removes the cgroup.
func (c *CgroupController) Cleanup() error {
	return os.RemoveAll(c.cgroupPath)
}

func (c *CgroupController) writeValue(file, value string) error {
	path := filepath.Join(c.cgroupPath, file)
	// Write with owner-only permissions where applicable to reduce exposure.
	// Note: many cgroup pseudo-files ignore mode bits, but keep conservative mode here.
	return os.WriteFile(path, []byte(value), 0600)
}

// ============================================================================
// Seccomp Filter Generation
// ============================================================================

// SeccompProfile represents a seccomp BPF profile.
type SeccompProfile struct {
	DefaultAction string `json:"defaultAction"`
	Syscalls      []Rule `json:"syscalls"`
}

// Rule represents a seccomp filter rule.
type Rule struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
}

// GenerateSeccompProfile creates a seccomp profile from a policy.
func GenerateSeccompProfile(policy *Policy) *SeccompProfile {
	profile := &SeccompProfile{
		DefaultAction: "SCMP_ACT_ALLOW",
		Syscalls:      []Rule{},
	}

	if policy.Syscalls.Mode == "deny" {
		profile.DefaultAction = "SCMP_ACT_KILL"

		if len(policy.Syscalls.AllowedSyscalls) > 0 {
			profile.Syscalls = append(profile.Syscalls, Rule{
				Names:  policy.Syscalls.AllowedSyscalls,
				Action: "SCMP_ACT_ALLOW",
			})
		}
	} else {
		// Allow mode - block specific syscalls
		if len(policy.Syscalls.DeniedSyscalls) > 0 {
			profile.Syscalls = append(profile.Syscalls, Rule{
				Names:  policy.Syscalls.DeniedSyscalls,
				Action: "SCMP_ACT_ERRNO",
			})
		}
	}

	return profile
}

// DefaultDeniedSyscalls returns a list of syscalls that are typically denied.
func DefaultDeniedSyscalls() []string {
	return []string{
		// Kernel/system modification
		"reboot",
		"sethostname",
		"setdomainname",
		"kexec_load",
		"kexec_file_load",

		// Module loading
		"init_module",
		"finit_module",
		"delete_module",

		// Mount operations
		"mount",
		"umount2",
		"pivot_root",

		// Namespace manipulation (unless explicitly allowed)
		"unshare",
		"setns",

		// Process tracing
		"ptrace",
		"process_vm_readv",
		"process_vm_writev",

		// Clock manipulation
		"clock_settime",
		"settimeofday",
		"adjtimex",

		// Kernel keyring
		"add_key",
		"keyctl",
		"request_key",

		// Performance counters (potential side channel)
		"perf_event_open",

		// BPF (potential for sandbox escape)
		"bpf",

		// User/group manipulation
		"setuid",
		"setgid",
		"setreuid",
		"setregid",
		"setresuid",
		"setresgid",
		"setgroups",

		// Quotas
		"quotactl",

		// Swap
		"swapon",
		"swapoff",

		// Filesystem operations
		"chroot",
		"acct",
	}
}
