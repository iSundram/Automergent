package tools

import "testing"

func TestScopeArtifactPath(t *testing.T) {
	cases := []struct{ path, session, want string }{
		{".automergent/artifacts/plan.md", "s1", ".automergent/artifacts/s1/plan.md"},
		{".automergent/artifacts/tui-upgrade-plan.md", "s1", ".automergent/artifacts/s1/tui-upgrade-plan.md"},
		// Already scoped to the same session: stable no-op.
		{".automergent/artifacts/s1/plan.md", "s1", ".automergent/artifacts/s1/plan.md"},
		// Different session: re-scoped.
		{".automergent/artifacts/s1/plan.md", "s2", ".automergent/artifacts/s2/s1/plan.md"},
		// Unnormalized input.
		{".automergent/artifacts//x.md", "s1", ".automergent/artifacts/s1/x.md"},
		// Outside artifacts home: untouched.
		{"docs/plan.md", "s1", "docs/plan.md"},
		{"internal/tui/app.go", "s1", "internal/tui/app.go"},
		// Absolute path: untouched.
		{"/tmp/.automergent/artifacts/plan.md", "s1", "/tmp/.automergent/artifacts/plan.md"},
		// No session or empty path: untouched.
		{"", "s1", ""},
		{".automergent/artifacts/plan.md", "", ".automergent/artifacts/plan.md"},
		// The bare artifacts dir itself: untouched.
		{".automergent/artifacts", "s1", ".automergent/artifacts"},
	}
	for _, tc := range cases {
		if got := ScopeArtifactPath(tc.path, tc.session); got != tc.want {
			t.Errorf("ScopeArtifactPath(%q, %q) = %q, want %q", tc.path, tc.session, got, tc.want)
		}
	}
}
