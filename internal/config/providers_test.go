package config

import (
	"sort"
	"testing"
)

func TestProviderNames(t *testing.T) {
	names := ProviderNames()
	if len(names) == 0 {
		t.Fatal("ProviderNames() returned empty")
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("ProviderNames() not sorted: %v", names)
	}
}

func TestIsKnownProvider(t *testing.T) {
	if !IsKnownProvider("google") {
		t.Error("google should be known")
	}
	if IsKnownProvider("nonexistent") {
		t.Error("nonexistent should not be known")
	}
}

func TestProviderSpecFor(t *testing.T) {
	spec, ok := ProviderSpecFor("google")
	if !ok {
		t.Fatal("ProviderSpecFor(google) returned false")
	}
	if spec.Name != "google" {
		t.Errorf("Name = %q, want google", spec.Name)
	}
	if spec.DefaultModel == "" {
		t.Error("DefaultModel should not be empty")
	}
	if spec.DisplayName == "" {
		t.Error("DisplayName should not be empty")
	}
	if spec.Icon == "" {
		t.Error("Icon should not be empty")
	}
	_, ok = ProviderSpecFor("nonexistent")
	if ok {
		t.Error("nonexistent should return false")
	}
}

func TestDefaultModelFor(t *testing.T) {
	m := DefaultModelFor("google")
	if m == "" {
		t.Error("DefaultModelFor(google) should not be empty")
	}
	m = DefaultModelFor("nonexistent")
	if m != "" {
		t.Errorf("DefaultModelFor(nonexistent) = %q, want empty", m)
	}
}

func TestProviderEnvKeys(t *testing.T) {
	keys := ProviderEnvKeys("google")
	if len(keys) == 0 {
		t.Error("ProviderEnvKeys(google) should not be empty")
	}
	keys = ProviderEnvKeys("nonexistent")
	if keys != nil {
		t.Errorf("ProviderEnvKeys(nonexistent) = %v, want nil", keys)
	}
}

func TestProviderIcon(t *testing.T) {
	icon := ProviderIcon("google")
	if icon == "" {
		t.Error("ProviderIcon(google) should not be empty")
	}
	icon = ProviderIcon("nonexistent")
	if icon != "🔌" {
		t.Errorf("ProviderIcon(nonexistent) = %q, want 🔌", icon)
	}
}

func TestIsValidBackend(t *testing.T) {
	if !IsValidBackend("google", "") {
		t.Error("empty backend should be valid")
	}
	if !IsValidBackend("google", "aistudio") {
		t.Error("aistudio should be valid for google")
	}
	if !IsValidBackend("google", "vertex") {
		t.Error("vertex should be valid for google")
	}
	if IsValidBackend("google", "bedrock") {
		t.Error("bedrock should not be valid for google")
	}
	if IsValidBackend("nonexistent", "aistudio") {
		t.Error("nonexistent provider should have no valid backends")
	}
}

func TestDefaultBackend(t *testing.T) {
	db := DefaultBackend("google")
	if db != "aistudio" {
		t.Errorf("DefaultBackend(google) = %q, want aistudio", db)
	}
	db = DefaultBackend("nonexistent")
	if db != "" {
		t.Errorf("DefaultBackend(nonexistent) = %q, want empty", db)
	}
}

func TestEffectiveBackend(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		pc       ProviderConfig
		want     string
	}{
		{"explicit backend", "google", ProviderConfig{Backend: "vertex"}, "vertex"},
		{"inferred vertex", "google", ProviderConfig{Project: "my-proj", Location: "us-central1"}, "vertex"},
		{"defaults to aistudio", "google", ProviderConfig{}, "aistudio"},
		{"unknown provider", "unknown", ProviderConfig{}, "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveBackend(tt.provider, tt.pc)
			if got != tt.want {
				t.Errorf("EffectiveBackend() = %q, want %q", got, tt.want)
			}
		})
	}
}
