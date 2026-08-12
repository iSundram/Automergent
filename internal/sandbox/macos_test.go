//go:build darwin

package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestMacOSSandboxCreation(t *testing.T) {
	sandbox := newPlatformSandbox("auto")

	if sandbox == nil {
		t.Fatal("newPlatformSandbox(auto) returned nil")
	}

	name := sandbox.Name()
	if name != "macos-seatbelt" && name != "noop" {
		t.Errorf("unexpected sandbox name: %s", name)
	}
}

func TestMacOSSandboxAvailability(t *testing.T) {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec not available")
	}

	sandbox := newPlatformSandbox("seatbelt")

	if sandbox.Name() != "macos-seatbelt" {
		t.Errorf("Name() = %q, want macos-seatbelt", sandbox.Name())
	}

	if !sandbox.IsAvailable() {
		t.Error("seatbelt sandbox should be available")
	}
}

func TestMacOSSandboxCapabilities(t *testing.T) {
	sandbox := &macOSSandbox{sandboxExecPath: "/usr/bin/sandbox-exec"}

	caps := sandbox.Capabilities()

	if !caps.SupportsNetworkIsolation {
		t.Error("seatbelt should support network isolation")
	}

	if !caps.SupportsFSIsolation {
		t.Error("seatbelt should support filesystem isolation")
	}

	if caps.SupportsSeccomp {
		t.Error("seatbelt should not claim seccomp support")
	}

	if caps.RequiresRoot {
		t.Error("seatbelt should not require root")
	}
}

func TestMacOSSandboxWrapCommand(t *testing.T) {
	sandbox := &macOSSandbox{sandboxExecPath: "/usr/bin/sandbox-exec"}
	ctx := context.Background()

	cmd, args := sandbox.Wrap(ctx, "echo", []string{"hello"})

	if cmd != "/usr/bin/sandbox-exec" {
		t.Errorf("Wrap() cmd = %q, want /usr/bin/sandbox-exec", cmd)
	}

	if len(args) < 3 {
		t.Fatalf("Wrap() args too short: %v", args)
	}

	if args[0] != "-p" {
		t.Errorf("first arg = %q, want -p", args[0])
	}

	// Profile should be in args[1]
	profile := args[1]
	if !strings.Contains(profile, "(version 1)") {
		t.Error("profile should contain version declaration")
	}

	// Original command should be args[2]
	if args[2] != "echo" {
		t.Errorf("args[2] = %q, want echo", args[2])
	}

	// Original args should follow
	if args[3] != "hello" {
		t.Errorf("args[3] = %q, want hello", args[3])
	}
}

func TestMacOSSandboxStrictPolicy(t *testing.T) {
	sandbox := &macOSSandbox{sandboxExecPath: "/usr/bin/sandbox-exec"}
	ctx := context.Background()
	policy := StrictPolicy()

	_, args := sandbox.WrapWithPolicy(ctx, "test", []string{}, policy)

	profile := args[1]

	// Strict policy should use deny default
	if !strings.Contains(profile, "(deny default)") {
		t.Error("strict policy should use deny default")
	}

	// Should deny network
	if !strings.Contains(profile, "(deny network*)") {
		t.Error("strict policy should deny network")
	}
}

func TestMacOSSandboxNetworkPolicy(t *testing.T) {
	sandbox := &macOSSandbox{sandboxExecPath: "/usr/bin/sandbox-exec"}
	ctx := context.Background()
	policy := NetworkPolicy_()

	_, args := sandbox.WrapWithPolicy(ctx, "test", []string{}, policy)

	profile := args[1]

	// Network policy should allow network
	if !strings.Contains(profile, "(allow network*)") {
		t.Error("network policy should allow network")
	}
}

func TestMacOSSandboxDevelopmentPolicy(t *testing.T) {
	sandbox := &macOSSandbox{sandboxExecPath: "/usr/bin/sandbox-exec"}
	ctx := context.Background()
	policy := DevelopmentPolicy()

	_, args := sandbox.WrapWithPolicy(ctx, "test", []string{}, policy)

	profile := args[1]

	// Development policy should use allow default
	if !strings.Contains(profile, "(allow default)") {
		t.Error("development policy should use allow default")
	}
}

func TestMacOSSandboxCustomPaths(t *testing.T) {
	sandbox := &macOSSandbox{sandboxExecPath: "/usr/bin/sandbox-exec"}
	ctx := context.Background()
	policy := StandardPolicy()
	policy.FileSystem.WorkDir = "/custom/work"
	policy.FileSystem.ReadWritePaths = append(policy.FileSystem.ReadWritePaths, "/custom/rw")

	_, args := sandbox.WrapWithPolicy(ctx, "test", []string{}, policy)

	profile := args[1]

	// Should allow write to work dir
	if !strings.Contains(profile, "/custom/work") {
		t.Error("profile should include work directory")
	}

	// Should allow write to custom rw path
	if !strings.Contains(profile, "/custom/rw") {
		t.Error("profile should include custom read-write path")
	}
}

func TestSeatbeltProfileGeneration(t *testing.T) {
	sandbox := &macOSSandbox{sandboxExecPath: "/usr/bin/sandbox-exec"}

	tests := []struct {
		name     string
		policy   *Policy
		contains []string
		excludes []string
	}{
		{
			name:   "strict",
			policy: StrictPolicy(),
			contains: []string{
				"(deny default)",
				"(deny network*)",
			},
		},
		{
			name:   "standard",
			policy: StandardPolicy(),
			contains: []string{
				"(allow default)",
				"(deny network*)",
			},
		},
		{
			name:   "permissive",
			policy: PermissivePolicy(),
			contains: []string{
				"(allow default)",
				"(allow network*)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := sandbox.generateSeatbeltProfile(tt.policy)

			for _, expected := range tt.contains {
				if !strings.Contains(profile, expected) {
					t.Errorf("profile should contain %q", expected)
				}
			}

			for _, excluded := range tt.excludes {
				if strings.Contains(profile, excluded) {
					t.Errorf("profile should not contain %q", excluded)
				}
			}
		})
	}
}

func TestPredefinedProfiles(t *testing.T) {
	profiles := PredefinedProfiles()

	if len(profiles) == 0 {
		t.Error("PredefinedProfiles() should return profiles")
	}

	for _, profile := range profiles {
		if profile.Name == "" {
			t.Error("profile should have name")
		}

		if profile.Content == "" {
			t.Error("profile should have content")
		}

		if !strings.Contains(profile.Content, "(version 1)") {
			t.Errorf("profile %q should contain version declaration", profile.Name)
		}
	}
}

func TestMacOSResourceLimits(t *testing.T) {
	limits := &MacOSResourceLimits{
		CPUTime:      60,
		FileSize:     1024 * 1024 * 100,
		DataSize:     1024 * 1024 * 512,
		OpenFiles:    1024,
		MaxProcesses: 100,
	}

	// Just verify struct creation
	if limits.CPUTime != 60 {
		t.Errorf("CPUTime = %d, want 60", limits.CPUTime)
	}

	if limits.MaxProcesses != 100 {
		t.Errorf("MaxProcesses = %d, want 100", limits.MaxProcesses)
	}
}

func TestMacOSNoneMode(t *testing.T) {
	sandbox := newPlatformSandbox("none")

	if sandbox.Name() != "noop" {
		t.Errorf("Name() = %q, want noop", sandbox.Name())
	}
}
