package shell

import (
	"testing"
)

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*_SECRET", "MY_SECRET", true},
		{"*_SECRET", "MY_KEY", false},
		{"AWS_*", "AWS_ACCESS_KEY_ID", true},
		{"AWS_*", "GEMINI_KEY", false},
		{"*", "ANYTHING", true},
		{"GEMINI_API_KEY", "GEMINI_API_KEY", true},
		{"GEMINI_API_KEY", "GEMINI_API_KEY_OLD", false},
		{"HTTP_PROXY", "HTTP_PROXY", true},
		{"HTTP_PROXY", "HTTPS_PROXY", false},
	}
	for _, tt := range tests {
		got := matchGlob(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestFilterEnv(t *testing.T) {
	env := []string{
		"HOME=/home/user",
		"GEMINI_API_KEY=sk-secret",
		"MY_PASSWORD=hunter2",
		"PATH=/usr/bin",
		"AWS_SECRET_ACCESS_KEY=awssecret",
		"NORMAL_VAR=value",
	}
	patterns := []string{"GEMINI_*", "*_PASSWORD", "AWS_SECRET_*"}

	result := filterEnv(env, patterns)

	// Should keep HOME, PATH, NORMAL_VAR
	allowed := map[string]bool{
		"HOME=/home/user":  true,
		"PATH=/usr/bin":    true,
		"NORMAL_VAR=value": true,
	}

	if len(result) != 3 {
		t.Errorf("expected 3 vars after filter, got %d: %v", len(result), result)
	}
	for _, v := range result {
		if !allowed[v] {
			t.Errorf("unexpected env var in result: %s", v)
		}
	}

	// Sensitive vars must be absent.
	sensitive := map[string]bool{
		"GEMINI_API_KEY=sk-secret":        true,
		"MY_PASSWORD=hunter2":             true,
		"AWS_SECRET_ACCESS_KEY=awssecret": true,
	}
	for _, v := range result {
		if sensitive[v] {
			t.Errorf("sensitive env var leaked: %s", v)
		}
	}
}
