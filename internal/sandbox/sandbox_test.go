package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPolicyPresets(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		wantErr bool
	}{
		{"strict policy", "strict", false},
		{"standard policy", "standard", false},
		{"permissive policy", "permissive", false},
		{"network policy", "network", false},
		{"development policy", "development", false},
		{"unknown policy", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := Preset(tt.preset)
			if (err != nil) != tt.wantErr {
				t.Errorf("Preset(%q) error = %v, wantErr %v", tt.preset, err, tt.wantErr)
				return
			}
			if !tt.wantErr && policy == nil {
				t.Errorf("Preset(%q) returned nil policy", tt.preset)
			}
			if !tt.wantErr && policy.Name != tt.preset {
				t.Errorf("Preset(%q) name = %q, want %q", tt.preset, policy.Name, tt.preset)
			}
		})
	}
}

func TestStrictPolicy(t *testing.T) {
	policy := StrictPolicy()

	if policy.Network.Enabled {
		t.Error("strict policy should disable network")
	}

	if policy.Process.AllowFork {
		t.Error("strict policy should disable fork")
	}

	if policy.Process.AllowExec {
		t.Error("strict policy should disable exec")
	}

	if policy.Resources.MemoryBytes != 256*1024*1024 {
		t.Errorf("strict policy memory limit = %d, want %d", policy.Resources.MemoryBytes, 256*1024*1024)
	}
}

func TestStandardPolicy(t *testing.T) {
	policy := StandardPolicy()

	if policy.Network.Enabled {
		t.Error("standard policy should disable network by default")
	}

	if !policy.Process.AllowFork {
		t.Error("standard policy should allow fork")
	}

	if !policy.Process.AllowExec {
		t.Error("standard policy should allow exec")
	}
}

func TestPolicyValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Policy)
		wantErr bool
	}{
		{
			name:    "valid policy",
			modify:  func(p *Policy) {},
			wantErr: false,
		},
		{
			name:    "empty name",
			modify:  func(p *Policy) { p.Name = "" },
			wantErr: true,
		},
		{
			name:    "negative memory",
			modify:  func(p *Policy) { p.Resources.MemoryBytes = -1 },
			wantErr: true,
		},
		{
			name:    "invalid CPU quota",
			modify:  func(p *Policy) { p.Resources.CPUQuota = 150 },
			wantErr: true,
		},
		{
			name:    "invalid IO weight",
			modify:  func(p *Policy) { p.Resources.IOWeight = 2000 },
			wantErr: true,
		},
		{
			name:    "invalid syscall mode",
			modify:  func(p *Policy) { p.Syscalls.Mode = "invalid" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := StandardPolicy()
			tt.modify(policy)
			err := policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPolicyWithModifiers(t *testing.T) {
	policy := StandardPolicy()

	// Test WithWorkDir
	modified := policy.WithWorkDir("/test/dir")
	if modified.FileSystem.WorkDir != "/test/dir" {
		t.Errorf("WithWorkDir() WorkDir = %q, want %q", modified.FileSystem.WorkDir, "/test/dir")
	}
	if !contains(modified.FileSystem.ReadWritePaths, "/test/dir") {
		t.Error("WithWorkDir() should add dir to ReadWritePaths")
	}

	// Test WithNetwork
	modified = policy.WithNetwork(true)
	if !modified.Network.Enabled {
		t.Error("WithNetwork(true) should enable network")
	}
	if !modified.Network.AllowDNS {
		t.Error("WithNetwork(true) should enable DNS")
	}

	// Test WithTimeout
	modified = policy.WithTimeout(5 * time.Minute)
	if modified.Resources.WallTime != 5*time.Minute {
		t.Errorf("WithTimeout() WallTime = %v, want %v", modified.Resources.WallTime, 5*time.Minute)
	}
}

func TestPolicyMerge(t *testing.T) {
	base := StandardPolicy()
	override := &Policy{
		Name: "merged",
		FileSystem: FileSystemPolicy{
			ReadOnlyPaths:  []string{"/custom/ro"},
			ReadWritePaths: []string{"/custom/rw"},
		},
		Network: NetworkPolicy{
			Enabled: false,
		},
		Resources: ResourceLimits{
			MemoryBytes: 128 * 1024 * 1024, // Lower than base
		},
	}

	merged := base.Merge(override)

	if merged.Name != "merged" {
		t.Errorf("Merge() Name = %q, want %q", merged.Name, "merged")
	}

	if !contains(merged.FileSystem.ReadOnlyPaths, "/custom/ro") {
		t.Error("Merge() should include override ReadOnlyPaths")
	}

	if merged.Resources.MemoryBytes != 128*1024*1024 {
		t.Errorf("Merge() MemoryBytes = %d, want %d", merged.Resources.MemoryBytes, 128*1024*1024)
	}
}

func TestSandboxNew(t *testing.T) {
	sandbox := New("auto")
	if sandbox == nil {
		t.Fatal("New() returned nil")
	}

	name := sandbox.Name()
	if name == "" {
		t.Error("sandbox.Name() returned empty string")
	}
}

func TestSandboxWrap(t *testing.T) {
	sandbox := New("auto")
	ctx := context.Background()

	cmd, args := sandbox.Wrap(ctx, "echo", []string{"hello"})

	// Should return something (either wrapped or unwrapped)
	if cmd == "" {
		t.Error("Wrap() returned empty command")
	}

	// The original command should be somewhere in the result
	allArgs := strings.Join(args, " ")
	if !strings.Contains(cmd+" "+allArgs, "echo") {
		t.Error("Wrap() result should contain original command")
	}
}

func TestSandboxCapabilities(t *testing.T) {
	sandbox := New("auto")

	caps := sandbox.Capabilities()

	// Description should never be empty
	if caps.Description == "" {
		t.Error("Capabilities() Description should not be empty")
	}
}

func TestNoopSandbox(t *testing.T) {
	sandbox := New("none")

	if sandbox.Name() != "noop" {
		t.Errorf("noop sandbox Name() = %q, want %q", sandbox.Name(), "noop")
	}

	if sandbox.IsAvailable() {
		t.Error("noop sandbox should not be available")
	}

	ctx := context.Background()
	cmd, args := sandbox.Wrap(ctx, "test", []string{"arg1", "arg2"})

	if cmd != "test" {
		t.Errorf("noop Wrap() cmd = %q, want %q", cmd, "test")
	}

	if len(args) != 2 || args[0] != "arg1" || args[1] != "arg2" {
		t.Errorf("noop Wrap() args = %v, want [arg1 arg2]", args)
	}
}

func TestAvailableSandboxes(t *testing.T) {
	available := Available()

	if len(available) == 0 {
		t.Error("Available() should return at least one sandbox type")
	}
}

func TestDescribeSandbox(t *testing.T) {
	desc, err := Describe("auto")
	if err != nil {
		t.Errorf("Describe(auto) error = %v", err)
	}
	if desc == "" {
		t.Error("Describe(auto) returned empty description")
	}
}

func TestSeccompProfile(t *testing.T) {
	policy := StrictPolicy()
	profile := GenerateSeccompProfile(policy)

	if profile == nil {
		t.Fatal("GenerateSeccompProfile() returned nil")
	}

	if profile.DefaultAction != "SCMP_ACT_KILL" {
		t.Errorf("strict policy DefaultAction = %q, want %q", profile.DefaultAction, "SCMP_ACT_KILL")
	}
}

func TestDefaultDeniedSyscalls(t *testing.T) {
	denied := DefaultDeniedSyscalls()

	if len(denied) == 0 {
		t.Error("DefaultDeniedSyscalls() should return syscalls")
	}

	// Check for some expected dangerous syscalls
	expected := []string{"reboot", "kexec_load", "init_module", "ptrace"}
	for _, syscall := range expected {
		if !contains(denied, syscall) {
			t.Errorf("DefaultDeniedSyscalls() should include %q", syscall)
		}
	}
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
