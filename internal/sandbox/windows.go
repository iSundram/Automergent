//go:build windows

package sandbox

import (
	"context"
	"fmt"
)

// windowsSandbox implements sandboxing on Windows.
type windowsSandbox struct {
	mode   string // "appcontainer", "job", or "integrity"
	policy *Policy
}

func newPlatformSandbox(kind string) Sandbox {
	switch kind {
	case "none", "off":
		return &noopSandbox{}
	case "appcontainer":
		return &windowsSandbox{mode: "appcontainer"}
	case "job":
		return &windowsSandbox{mode: "job"}
	case "integrity":
		return &windowsSandbox{mode: "integrity"}
	default: // "auto"
		// Windows sandboxing requires additional setup, default to noop
		// Users can explicitly enable it when needed
		return &noopSandbox{}
	}
}

func (s *windowsSandbox) Name() string {
	return fmt.Sprintf("windows-%s", s.mode)
}

func (s *windowsSandbox) IsAvailable() bool {
	// Windows sandboxing is available on Windows 10+ with proper APIs
	// This is a placeholder - full implementation would check for APIs
	return false
}

func (s *windowsSandbox) Capabilities() Capabilities {
	caps := Capabilities{
		RequiresRoot: false, // Windows uses tokens instead
		Description:  "Windows sandbox using ",
	}

	switch s.mode {
	case "appcontainer":
		caps.SupportsNetworkIsolation = true
		caps.SupportsFSIsolation = true
		caps.SupportsResourceLimits = false
		caps.Description += "AppContainer isolation"
	case "job":
		caps.SupportsResourceLimits = true
		caps.SupportsFSIsolation = false
		caps.SupportsNetworkIsolation = false
		caps.Description += "Job Objects for resource limits"
	case "integrity":
		caps.SupportsFSIsolation = true
		caps.SupportsNetworkIsolation = false
		caps.SupportsResourceLimits = false
		caps.Description += "integrity levels for access control"
	}

	return caps
}

func (s *windowsSandbox) Wrap(_ context.Context, name string, args []string) (string, []string) {
	return s.WrapWithPolicy(context.Background(), name, args, nil)
}

func (s *windowsSandbox) WrapWithPolicy(_ context.Context, name string, args []string, policy *Policy) (string, []string) {
	// Windows sandboxing is not implemented in this package on non-Windows builds.
	// IMPORTANT SECURITY NOTE:
	// - The current implementation is a no-op and will execute commands without
	//   additional Windows-specific isolation. Do NOT rely on this for security.
	// - For production use on Windows, implement one of the following:
	//   * Job Objects (golang.org/x/sys/windows): create a Job Object, set
	//     limits and JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, and assign child processes
	//     to it so they are terminated together.
	//   * AppContainer: use AppContainer APIs for stronger filesystem and network
	//     isolation for UWP-style sandboxes.
	//   * Restricted tokens/CreateRestrictedToken to remove privileges from the
	//     child process token.
	// Until a proper Windows implementation is added, this wrapper intentionally
	// returns the unmodified command and arguments so callers are aware no sandbox
	// protections are applied.
	return name, args
}

// WindowsJobLimits represents Job Object limits for Windows.
type WindowsJobLimits struct {
	// ActiveProcessLimit limits the number of processes in the job.
	ActiveProcessLimit int

	// ProcessMemoryLimit limits memory per process in bytes.
	ProcessMemoryLimit int64

	// JobMemoryLimit limits total job memory in bytes.
	JobMemoryLimit int64

	// PerProcessUserTimeLimit limits CPU time per process.
	PerProcessUserTimeLimit int64

	// PerJobUserTimeLimit limits total CPU time for the job.
	PerJobUserTimeLimit int64

	// LimitFlags contains additional job limit flags.
	LimitFlags uint32

	// UIRestrictions restricts UI interactions.
	UIRestrictions uint32
}

// Common limit flags for Windows Job Objects
const (
	JOB_OBJECT_LIMIT_ACTIVE_PROCESS      = 0x00000008
	JOB_OBJECT_LIMIT_PROCESS_MEMORY      = 0x00000100
	JOB_OBJECT_LIMIT_JOB_MEMORY          = 0x00000200
	JOB_OBJECT_LIMIT_PROCESS_TIME        = 0x00000002
	JOB_OBJECT_LIMIT_JOB_TIME            = 0x00000004
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE   = 0x00002000
	JOB_OBJECT_LIMIT_BREAKAWAY_OK        = 0x00000800
	JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK = 0x00001000
)

// UI restriction flags
const (
	JOB_OBJECT_UILIMIT_DESKTOP          = 0x00000040
	JOB_OBJECT_UILIMIT_DISPLAYSETTINGS  = 0x00000010
	JOB_OBJECT_UILIMIT_EXITWINDOWS      = 0x00000080
	JOB_OBJECT_UILIMIT_GLOBALATOMS      = 0x00000020
	JOB_OBJECT_UILIMIT_HANDLES          = 0x00000001
	JOB_OBJECT_UILIMIT_READCLIPBOARD    = 0x00000002
	JOB_OBJECT_UILIMIT_SYSTEMPARAMETERS = 0x00000008
	JOB_OBJECT_UILIMIT_WRITECLIPBOARD   = 0x00000004
)

// WindowsIntegrityLevel represents Windows integrity levels.
type WindowsIntegrityLevel int

const (
	IntegrityUntrusted WindowsIntegrityLevel = 0
	IntegrityLow       WindowsIntegrityLevel = 1
	IntegrityMedium    WindowsIntegrityLevel = 2
	IntegrityHigh      WindowsIntegrityLevel = 3
	IntegritySystem    WindowsIntegrityLevel = 4
)

// AppContainerProfile represents an AppContainer sandbox profile.
type AppContainerProfile struct {
	// Name is the AppContainer profile name.
	Name string

	// DisplayName is the human-readable name.
	DisplayName string

	// Description describes the profile.
	Description string

	// Capabilities are the granted capabilities.
	Capabilities []string

	// AllowedPaths are paths accessible from the container.
	AllowedPaths []string
}

// Common AppContainer capabilities
var AppContainerCapabilities = map[string]string{
	"internetClient":             "S-1-15-3-1",
	"internetClientServer":       "S-1-15-3-2",
	"privateNetworkClientServer": "S-1-15-3-3",
	"picturesLibrary":            "S-1-15-3-4",
	"videosLibrary":              "S-1-15-3-5",
	"musicLibrary":               "S-1-15-3-6",
	"documentsLibrary":           "S-1-15-3-7",
	"enterpriseAuthentication":   "S-1-15-3-8",
	"sharedUserCertificates":     "S-1-15-3-9",
	"removableStorage":           "S-1-15-3-10",
	"appointments":               "S-1-15-3-11",
	"contacts":                   "S-1-15-3-12",
}

// WindowsSandboxConfig provides Windows-specific sandbox configuration.
type WindowsSandboxConfig struct {
	// Mode is the sandbox mode: "appcontainer", "job", or "integrity"
	Mode string

	// IntegrityLevel sets the process integrity level.
	IntegrityLevel WindowsIntegrityLevel

	// JobLimits sets Job Object limits.
	JobLimits *WindowsJobLimits

	// AppContainer configures AppContainer isolation.
	AppContainer *AppContainerProfile

	// RestrictedToken enables token restriction.
	RestrictedToken bool

	// DisablePrivileges removes all privileges.
	DisablePrivileges bool

	// DisabledSids lists SIDs to disable in the token.
	DisabledSids []string

	// RestrictedSids lists restricting SIDs.
	RestrictedSids []string
}

// CreateJobLimits creates Windows Job Object limits from a policy.
func CreateJobLimits(policy *Policy) *WindowsJobLimits {
	if policy == nil {
		return nil
	}

	limits := &WindowsJobLimits{
		LimitFlags: JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
	}

	if policy.Process.MaxProcesses > 0 {
		limits.ActiveProcessLimit = policy.Process.MaxProcesses
		limits.LimitFlags |= JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	}

	if policy.Resources.MemoryBytes > 0 {
		limits.JobMemoryLimit = policy.Resources.MemoryBytes
		limits.LimitFlags |= JOB_OBJECT_LIMIT_JOB_MEMORY
	}

	if policy.Resources.CPUTime > 0 {
		// Convert to 100-nanosecond intervals
		limits.PerJobUserTimeLimit = int64(policy.Resources.CPUTime.Nanoseconds() / 100)
		limits.LimitFlags |= JOB_OBJECT_LIMIT_JOB_TIME
	}

	return limits
}

// Note: Full Windows implementation would use:
// - golang.org/x/sys/windows for Windows API calls
// - CreateRestrictedToken, SetTokenInformation for token manipulation
// - CreateJobObject, AssignProcessToJobObject for job objects
// - AppContainer APIs for UWP-style isolation
