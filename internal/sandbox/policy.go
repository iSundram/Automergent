package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Policy defines security constraints for sandboxed execution.
type Policy struct {
	// Name is a human-readable identifier for this policy.
	Name string `json:"name"`

	// FileSystem controls file access permissions.
	FileSystem FileSystemPolicy `json:"filesystem"`

	// Network controls network access.
	Network NetworkPolicy `json:"network"`

	// Process controls process-related restrictions.
	Process ProcessPolicy `json:"process"`

	// Resources defines resource limits.
	Resources ResourceLimits `json:"resources"`

	// Syscalls defines system call filtering rules.
	Syscalls SyscallPolicy `json:"syscalls"`
}

// FileSystemPolicy defines file system access rules.
type FileSystemPolicy struct {
	// ReadOnlyPaths are paths that can only be read.
	ReadOnlyPaths []string `json:"readonly_paths"`

	// ReadWritePaths are paths that can be read and written.
	ReadWritePaths []string `json:"readwrite_paths"`

	// DeniedPaths are paths that cannot be accessed at all.
	DeniedPaths []string `json:"denied_paths"`

	// TempDir specifies a temporary directory for the sandbox.
	TempDir string `json:"temp_dir"`

	// WorkDir specifies the working directory inside the sandbox.
	WorkDir string `json:"work_dir"`

	// HomeDir specifies the home directory inside the sandbox.
	HomeDir string `json:"home_dir"`

	// AllowDevices permits access to device files.
	AllowDevices bool `json:"allow_devices"`

	// AllowProc permits access to /proc.
	AllowProc bool `json:"allow_proc"`

	// AllowSys permits access to /sys.
	AllowSys bool `json:"allow_sys"`
}

// NetworkPolicy defines network access rules.
type NetworkPolicy struct {
	// Enabled controls whether network access is allowed at all.
	Enabled bool `json:"enabled"`

	// AllowedHosts lists hosts that can be connected to.
	AllowedHosts []string `json:"allowed_hosts"`

	// AllowedPorts lists ports that can be used.
	AllowedPorts []int `json:"allowed_ports"`

	// AllowLoopback permits localhost connections.
	AllowLoopback bool `json:"allow_loopback"`

	// AllowDNS permits DNS queries.
	AllowDNS bool `json:"allow_dns"`
}

// ProcessPolicy defines process-related restrictions.
type ProcessPolicy struct {
	// AllowFork permits creating child processes.
	AllowFork bool `json:"allow_fork"`

	// AllowExec permits executing other programs.
	AllowExec bool `json:"allow_exec"`

	// AllowedExecutables lists programs that can be executed.
	AllowedExecutables []string `json:"allowed_executables"`

	// MaxProcesses limits the number of processes.
	MaxProcesses int `json:"max_processes"`

	// AllowPtrace permits debugging/tracing.
	AllowPtrace bool `json:"allow_ptrace"`

	// NewSession creates a new session for the process.
	NewSession bool `json:"new_session"`

	// DropCapabilities lists Linux capabilities to drop.
	DropCapabilities []string `json:"drop_capabilities"`
}

// ResourceLimits defines resource consumption limits.
type ResourceLimits struct {
	// CPUTime is the maximum CPU time allowed.
	CPUTime time.Duration `json:"cpu_time"`

	// WallTime is the maximum wall-clock time allowed.
	WallTime time.Duration `json:"wall_time"`

	// MemoryBytes is the maximum memory in bytes.
	MemoryBytes int64 `json:"memory_bytes"`

	// MemorySwapBytes is the maximum memory+swap in bytes.
	MemorySwapBytes int64 `json:"memory_swap_bytes"`

	// DiskBytes is the maximum disk space in bytes.
	DiskBytes int64 `json:"disk_bytes"`

	// MaxOpenFiles limits the number of open file descriptors.
	MaxOpenFiles int `json:"max_open_files"`

	// MaxFileSize limits the size of created files.
	MaxFileSize int64 `json:"max_file_size"`

	// CPUShares controls CPU scheduling priority (cgroups).
	CPUShares int `json:"cpu_shares"`

	// CPUQuota limits CPU usage as percentage (e.g., 50 = 50%).
	CPUQuota int `json:"cpu_quota"`

	// IOWeight controls I/O priority (1-1000).
	IOWeight int `json:"io_weight"`
}

// SyscallPolicy defines system call filtering rules.
type SyscallPolicy struct {
	// Mode specifies the filtering mode: "allow", "deny", or "audit".
	Mode string `json:"mode"`

	// AllowedSyscalls lists system calls that are permitted.
	AllowedSyscalls []string `json:"allowed_syscalls"`

	// DeniedSyscalls lists system calls that are blocked.
	DeniedSyscalls []string `json:"denied_syscalls"`

	// DefaultAction is the action for syscalls not in any list.
	DefaultAction string `json:"default_action"`
}

// Preset returns a predefined policy by name.
func Preset(name string) (*Policy, error) {
	switch name {
	case "strict":
		return StrictPolicy(), nil
	case "standard":
		return StandardPolicy(), nil
	case "permissive":
		return PermissivePolicy(), nil
	case "network":
		return NetworkPolicy_(), nil
	case "development":
		return DevelopmentPolicy(), nil
	default:
		return nil, fmt.Errorf("unknown policy preset: %s", name)
	}
}

// StrictPolicy returns a highly restrictive policy for untrusted code.
func StrictPolicy() *Policy {
	return &Policy{
		Name: "strict",
		FileSystem: FileSystemPolicy{
			ReadOnlyPaths: []string{
				"/usr",
				"/lib",
				"/lib64",
				"/bin",
				"/sbin",
			},
			ReadWritePaths: []string{},
			DeniedPaths: []string{
				"/etc/passwd",
				"/etc/shadow",
				"/etc/ssh",
				"/root",
				"/home",
			},
			AllowDevices: false,
			AllowProc:    false,
			AllowSys:     false,
		},
		Network: NetworkPolicy{
			Enabled:       false,
			AllowLoopback: false,
			AllowDNS:      false,
		},
		Process: ProcessPolicy{
			AllowFork:        false,
			AllowExec:        false,
			MaxProcesses:     1,
			AllowPtrace:      false,
			NewSession:       true,
			DropCapabilities: []string{"ALL"},
		},
		Resources: ResourceLimits{
			CPUTime:      30 * time.Second,
			WallTime:     60 * time.Second,
			MemoryBytes:  256 * 1024 * 1024, // 256 MB
			DiskBytes:    100 * 1024 * 1024, // 100 MB
			MaxOpenFiles: 64,
			MaxFileSize:  10 * 1024 * 1024, // 10 MB
			CPUQuota:     50,
		},
		Syscalls: SyscallPolicy{
			Mode:          "deny",
			DefaultAction: "kill",
			AllowedSyscalls: []string{
				"read", "write", "close", "fstat", "lseek",
				"mmap", "mprotect", "munmap", "brk",
				"exit", "exit_group",
			},
		},
	}
}

// StandardPolicy returns a balanced policy for general use.
func StandardPolicy() *Policy {
	return &Policy{
		Name: "standard",
		FileSystem: FileSystemPolicy{
			ReadOnlyPaths: []string{
				"/usr",
				"/lib",
				"/lib64",
				"/bin",
				"/sbin",
				"/etc/alternatives",
				"/etc/ld.so.cache",
				"/etc/localtime",
			},
			ReadWritePaths: []string{},
			DeniedPaths: []string{
				"/etc/passwd",
				"/etc/shadow",
				"/etc/ssh",
			},
			AllowDevices: true,
			AllowProc:    true,
			AllowSys:     false,
		},
		Network: NetworkPolicy{
			Enabled:       false,
			AllowLoopback: true,
			AllowDNS:      false,
		},
		Process: ProcessPolicy{
			AllowFork:        true,
			AllowExec:        true,
			MaxProcesses:     64,
			AllowPtrace:      false,
			NewSession:       true,
			DropCapabilities: []string{"SYS_ADMIN", "NET_ADMIN", "SYS_PTRACE"},
		},
		Resources: ResourceLimits{
			CPUTime:      5 * time.Minute,
			WallTime:     10 * time.Minute,
			MemoryBytes:  1024 * 1024 * 1024, // 1 GB
			DiskBytes:    1024 * 1024 * 1024, // 1 GB
			MaxOpenFiles: 1024,
			MaxFileSize:  100 * 1024 * 1024, // 100 MB
			CPUQuota:     100,
		},
		Syscalls: SyscallPolicy{
			Mode:          "allow",
			DefaultAction: "allow",
			DeniedSyscalls: []string{
				"ptrace", "process_vm_readv", "process_vm_writev",
				"personality", "mount", "umount2",
				"pivot_root", "chroot",
				"reboot", "sethostname", "setdomainname",
				"kexec_load", "init_module", "finit_module", "delete_module",
			},
		},
	}
}

// PermissivePolicy returns a minimal restriction policy.
func PermissivePolicy() *Policy {
	return &Policy{
		Name: "permissive",
		FileSystem: FileSystemPolicy{
			ReadOnlyPaths: []string{
				"/usr",
				"/lib",
				"/lib64",
				"/bin",
				"/sbin",
				"/etc",
			},
			ReadWritePaths: []string{},
			DeniedPaths:    []string{},
			AllowDevices:   true,
			AllowProc:      true,
			AllowSys:       true,
		},
		Network: NetworkPolicy{
			Enabled:       true,
			AllowLoopback: true,
			AllowDNS:      true,
		},
		Process: ProcessPolicy{
			AllowFork:        true,
			AllowExec:        true,
			MaxProcesses:     256,
			AllowPtrace:      false,
			NewSession:       false,
			DropCapabilities: []string{"SYS_ADMIN"},
		},
		Resources: ResourceLimits{
			CPUTime:      30 * time.Minute,
			WallTime:     60 * time.Minute,
			MemoryBytes:  4 * 1024 * 1024 * 1024,  // 4 GB
			DiskBytes:    10 * 1024 * 1024 * 1024, // 10 GB
			MaxOpenFiles: 4096,
			MaxFileSize:  1024 * 1024 * 1024, // 1 GB
			CPUQuota:     100,
		},
		Syscalls: SyscallPolicy{
			Mode:          "allow",
			DefaultAction: "allow",
			DeniedSyscalls: []string{
				"reboot", "kexec_load", "init_module", "finit_module", "delete_module",
			},
		},
	}
}

// NetworkPolicy_ returns a policy that allows network access.
func NetworkPolicy_() *Policy {
	p := StandardPolicy()
	p.Name = "network"
	p.Network.Enabled = true
	p.Network.AllowDNS = true
	p.Network.AllowLoopback = true
	return p
}

// DevelopmentPolicy returns a policy suitable for development tasks.
func DevelopmentPolicy() *Policy {
	return &Policy{
		Name: "development",
		FileSystem: FileSystemPolicy{
			ReadOnlyPaths: []string{
				"/usr",
				"/lib",
				"/lib64",
				"/bin",
				"/sbin",
				"/etc",
			},
			ReadWritePaths: []string{},
			DeniedPaths:    []string{"/etc/shadow"},
			AllowDevices:   true,
			AllowProc:      true,
			AllowSys:       false,
		},
		Network: NetworkPolicy{
			Enabled:       true,
			AllowLoopback: true,
			AllowDNS:      true,
		},
		Process: ProcessPolicy{
			AllowFork:        true,
			AllowExec:        true,
			MaxProcesses:     512,
			AllowPtrace:      true,
			NewSession:       false,
			DropCapabilities: []string{},
		},
		Resources: ResourceLimits{
			CPUTime:      60 * time.Minute,
			WallTime:     120 * time.Minute,
			MemoryBytes:  8 * 1024 * 1024 * 1024,  // 8 GB
			DiskBytes:    50 * 1024 * 1024 * 1024, // 50 GB
			MaxOpenFiles: 8192,
			MaxFileSize:  5 * 1024 * 1024 * 1024, // 5 GB
			CPUQuota:     100,
		},
		Syscalls: SyscallPolicy{
			Mode:          "allow",
			DefaultAction: "allow",
			DeniedSyscalls: []string{
				"reboot", "kexec_load", "init_module",
			},
		},
	}
}

// LoadPolicy loads a policy from a JSON file.
func LoadPolicy(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("parsing policy: %w", err)
	}

	return &policy, nil
}

// SavePolicy saves a policy to a JSON file.
func (p *Policy) SavePolicy(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling policy: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing policy file: %w", err)
	}

	return nil
}

// Validate checks if the policy configuration is valid.
func (p *Policy) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("policy name is required")
	}

	if p.Resources.MemoryBytes < 0 {
		return fmt.Errorf("memory limit cannot be negative")
	}

	if p.Resources.CPUQuota < 0 || p.Resources.CPUQuota > 100 {
		return fmt.Errorf("CPU quota must be between 0 and 100")
	}

	if p.Resources.IOWeight < 0 || p.Resources.IOWeight > 1000 {
		return fmt.Errorf("IO weight must be between 0 and 1000")
	}

	if p.Syscalls.Mode != "" && p.Syscalls.Mode != "allow" && p.Syscalls.Mode != "deny" && p.Syscalls.Mode != "audit" {
		return fmt.Errorf("invalid syscall mode: %s", p.Syscalls.Mode)
	}

	return nil
}

// Merge combines this policy with another, with the other taking precedence.
func (p *Policy) Merge(other *Policy) *Policy {
	merged := *p

	if other.Name != "" {
		merged.Name = other.Name
	}

	// Merge file system policy
	if len(other.FileSystem.ReadOnlyPaths) > 0 {
		merged.FileSystem.ReadOnlyPaths = append(merged.FileSystem.ReadOnlyPaths, other.FileSystem.ReadOnlyPaths...)
	}
	if len(other.FileSystem.ReadWritePaths) > 0 {
		merged.FileSystem.ReadWritePaths = append(merged.FileSystem.ReadWritePaths, other.FileSystem.ReadWritePaths...)
	}
	if len(other.FileSystem.DeniedPaths) > 0 {
		merged.FileSystem.DeniedPaths = append(merged.FileSystem.DeniedPaths, other.FileSystem.DeniedPaths...)
	}

	// Merge network policy (more restrictive wins)
	if !other.Network.Enabled {
		merged.Network.Enabled = false
	}

	// Merge resource limits (lower wins)
	if other.Resources.MemoryBytes > 0 && other.Resources.MemoryBytes < merged.Resources.MemoryBytes {
		merged.Resources.MemoryBytes = other.Resources.MemoryBytes
	}
	if other.Resources.CPUTime > 0 && other.Resources.CPUTime < merged.Resources.CPUTime {
		merged.Resources.CPUTime = other.Resources.CPUTime
	}

	return &merged
}

// WithWorkDir returns a copy of the policy with the specified working directory.
func (p *Policy) WithWorkDir(dir string) *Policy {
	copy := *p
	copy.FileSystem.WorkDir = dir
	copy.FileSystem.ReadWritePaths = append(copy.FileSystem.ReadWritePaths, dir)
	return &copy
}

// WithNetwork returns a copy of the policy with network settings modified.
func (p *Policy) WithNetwork(enabled bool) *Policy {
	copy := *p
	copy.Network.Enabled = enabled
	if enabled {
		copy.Network.AllowDNS = true
	}
	return &copy
}

// WithTimeout returns a copy of the policy with the specified timeout.
func (p *Policy) WithTimeout(d time.Duration) *Policy {
	copy := *p
	copy.Resources.WallTime = d
	return &copy
}
