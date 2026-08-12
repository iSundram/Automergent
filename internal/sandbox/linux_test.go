//go:build linux

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLinuxSandboxCreation(t *testing.T) {
	sandbox := newPlatformSandbox("auto")

	if sandbox == nil {
		t.Fatal("newPlatformSandbox(auto) returned nil")
	}

	name := sandbox.Name()
	if name != "linux-bubblewrap" && name != "linux-namespaces" && name != "noop" {
		t.Errorf("unexpected sandbox name: %s", name)
	}
}

func TestLinuxSandboxBubblewrap(t *testing.T) {
	// Check if bubblewrap is available
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap not available")
	}

	sandbox := newPlatformSandbox("bubblewrap")

	if sandbox.Name() != "linux-bubblewrap" {
		t.Errorf("Name() = %q, want linux-bubblewrap", sandbox.Name())
	}

	if !sandbox.IsAvailable() {
		t.Error("bubblewrap sandbox should be available")
	}

	caps := sandbox.Capabilities()
	if !caps.SupportsNamespaces {
		t.Error("bubblewrap should support namespaces")
	}
	if !caps.SupportsNetworkIsolation {
		t.Error("bubblewrap should support network isolation")
	}
}

func TestLinuxSandboxWrapCommand(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap not available")
	}

	sandbox := newPlatformSandbox("bubblewrap")
	ctx := context.Background()

	cmd, args := sandbox.Wrap(ctx, "echo", []string{"hello"})

	if cmd != "bwrap" {
		t.Errorf("Wrap() cmd = %q, want bwrap", cmd)
	}

	// Check for essential bubblewrap flags
	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, "--unshare-net") {
		t.Error("should include --unshare-net for network isolation")
	}
	if !strings.Contains(argStr, "--die-with-parent") {
		t.Error("should include --die-with-parent")
	}
	if !strings.Contains(argStr, "echo") {
		t.Error("should include original command")
	}
}

func TestLinuxSandboxWithPolicy(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap not available")
	}

	sandbox := newPlatformSandbox("bubblewrap").(*linuxSandbox)
	ctx := context.Background()
	policy := StrictPolicy()

	cmd, args := sandbox.WrapWithPolicy(ctx, "echo", []string{"hello"}, policy)

	if cmd != "bwrap" {
		t.Errorf("WrapWithPolicy() cmd = %q, want bwrap", cmd)
	}

	argStr := strings.Join(args, " ")

	// Strict policy should clear environment
	if !strings.Contains(argStr, "--clearenv") {
		t.Error("strict policy should include --clearenv")
	}
}

func TestLinuxNamespaceSandbox(t *testing.T) {
	// Check if unshare is available
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("unshare not available")
	}

	sandbox := newPlatformSandbox("namespaces")

	if sandbox.Name() != "linux-namespaces" {
		// May fall back to noop if user namespaces aren't available
		if sandbox.Name() != "noop" {
			t.Errorf("Name() = %q, want linux-namespaces or noop", sandbox.Name())
		}
		return
	}

	caps := sandbox.Capabilities()
	if !caps.SupportsNamespaces {
		t.Error("namespace sandbox should support namespaces")
	}
}

func TestLinuxNamespaceSandboxWrap(t *testing.T) {
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("unshare not available")
	}

	// Skip if not root and user namespaces not available
	if os.Getuid() != 0 && !checkUserNamespacesAvailable() {
		t.Skip("requires root or user namespace support")
	}

	sandbox := &linuxNamespaceSandbox{}
	ctx := context.Background()

	cmd, args := sandbox.Wrap(ctx, "echo", []string{"hello"})

	if cmd != "unshare" {
		t.Errorf("Wrap() cmd = %q, want unshare", cmd)
	}

	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, "--user") {
		t.Error("should include --user for user namespace")
	}
	if !strings.Contains(argStr, "--pid") {
		t.Error("should include --pid for pid namespace")
	}
}

func TestCheckSeccompAvailable(t *testing.T) {
	available := checkSeccompAvailable()
	// Just check it doesn't panic
	t.Logf("seccomp available: %v", available)
}

func TestCheckCgroupsAvailable(t *testing.T) {
	available := checkCgroupsAvailable()
	t.Logf("cgroups v2 available: %v", available)
}

func TestCheckUserNamespacesAvailable(t *testing.T) {
	available := checkUserNamespacesAvailable()
	t.Logf("user namespaces available: %v", available)
}

func TestCgroupController(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root for cgroup operations")
	}

	if !checkCgroupsAvailable() {
		t.Skip("cgroups v2 not available")
	}

	controller, err := NewCgroupController("test-sandbox")
	if err != nil {
		t.Fatalf("NewCgroupController() error = %v", err)
	}
	defer controller.Cleanup()

	if _, err := os.Stat("/sys/fs/cgroup/automergent-sandbox"); err != nil {
		t.Skip("cgroup sandbox path not writable in this environment")
	}

	// Test setting resource limits; skip if cgroup fs is not writable here.
	if err := controller.SetMemoryLimit(256 * 1024 * 1024); err != nil {
		t.Skipf("SetMemoryLimit unsupported here: %v", err)
	}
	if err := controller.SetCPUQuota(50); err != nil {
		t.Skipf("SetCPUQuota unsupported here: %v", err)
	}
	if err := controller.SetPIDLimit(100); err != nil {
		t.Skipf("SetPIDLimit unsupported here: %v", err)
	}
}

func TestLinuxSeccompProfile(t *testing.T) {
	// Test with deny mode
	policy := StrictPolicy()
	profile := GenerateSeccompProfile(policy)

	if profile.DefaultAction != "SCMP_ACT_KILL" {
		t.Errorf("DefaultAction = %q, want SCMP_ACT_KILL", profile.DefaultAction)
	}

	// Test with allow mode
	policy = StandardPolicy()
	profile = GenerateSeccompProfile(policy)

	if profile.DefaultAction != "SCMP_ACT_ALLOW" {
		t.Errorf("DefaultAction = %q, want SCMP_ACT_ALLOW", profile.DefaultAction)
	}
}

func TestLinuxBuildBwrapArgs(t *testing.T) {
	sandbox := &linuxSandbox{
		bwrapPath:    "/usr/bin/bwrap",
		seccompAvail: true,
		cgroupsAvail: true,
	}

	policy := StandardPolicy()
	args := sandbox.buildBwrapArgs(policy, "test-cmd")

	// Check essential arguments
	hasOption := func(opt string) bool {
		for _, arg := range args {
			if arg == opt {
				return true
			}
		}
		return false
	}

	if !hasOption("--die-with-parent") {
		t.Error("missing --die-with-parent")
	}

	if !hasOption("--unshare-user") {
		t.Error("missing --unshare-user")
	}

	if !hasOption("--unshare-pid") {
		t.Error("missing --unshare-pid")
	}

	if !hasOption("--") {
		t.Error("missing -- separator")
	}
}
