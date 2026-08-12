package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile represents a named configuration profile.
type Profile struct {
	// Name is the profile identifier.
	Name string `yaml:"name" json:"name"`
	// Description explains the profile's purpose.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Inherits specifies parent profile(s) to inherit from.
	Inherits []string `yaml:"inherits,omitempty" json:"inherits,omitempty"`
	// Environment specifies when this profile should auto-activate.
	Environment string `yaml:"environment,omitempty" json:"environment,omitempty"`
	// Config holds the profile-specific configuration overrides.
	Config map[string]any `yaml:"config" json:"config"`
	// Metadata holds additional profile information.
	Metadata ProfileMetadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// ProfileMetadata holds profile metadata.
type ProfileMetadata struct {
	// Author is the profile creator.
	Author string `yaml:"author,omitempty" json:"author,omitempty"`
	// Version is the profile version.
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
	// Created is when the profile was created.
	Created string `yaml:"created,omitempty" json:"created,omitempty"`
	// Tags are searchable tags.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// ProfileManager handles profile operations.
type ProfileManager struct {
	profilesDir string
	profiles    map[string]*Profile
}

// NewProfileManager creates a new profile manager.
func NewProfileManager() (*ProfileManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	profilesDir := filepath.Join(home, ".automergent", "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create profiles dir: %w", err)
	}

	pm := &ProfileManager{
		profilesDir: profilesDir,
		profiles:    make(map[string]*Profile),
	}

	// Load existing profiles
	if err := pm.loadProfiles(); err != nil {
		return nil, err
	}

	return pm, nil
}

// loadProfiles loads all profiles from disk.
func (pm *ProfileManager) loadProfiles() error {
	entries, err := os.ReadDir(pm.profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		path := filepath.Join(pm.profilesDir, entry.Name())
		profile, err := pm.loadProfile(path)
		if err != nil {
			continue // Skip invalid profiles
		}

		pm.profiles[profile.Name] = profile
	}

	return nil
}

// loadProfile loads a single profile from a file.
func (pm *ProfileManager) loadProfile(path string) (*Profile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var profile Profile
	if err := yaml.Unmarshal(content, &profile); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}

	// Use filename as name if not specified
	if profile.Name == "" {
		base := filepath.Base(path)
		profile.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return &profile, nil
}

// Get returns a profile by name.
func (pm *ProfileManager) Get(name string) (*Profile, error) {
	profile, ok := pm.profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	return profile, nil
}

// List returns all available profiles.
func (pm *ProfileManager) List() []*Profile {
	profiles := make([]*Profile, 0, len(pm.profiles))
	for _, p := range pm.profiles {
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles
}

// ListNames returns all profile names.
func (pm *ProfileManager) ListNames() []string {
	names := make([]string, 0, len(pm.profiles))
	for name := range pm.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Create creates a new profile.
func (pm *ProfileManager) Create(profile *Profile) error {
	if profile.Name == "" {
		return fmt.Errorf("profile name is required")
	}

	if _, exists := pm.profiles[profile.Name]; exists {
		return fmt.Errorf("profile %q already exists", profile.Name)
	}

	// Validate inheritance
	for _, parent := range profile.Inherits {
		if _, ok := pm.profiles[parent]; !ok {
			return fmt.Errorf("parent profile %q not found", parent)
		}
	}

	// Save to disk
	if err := pm.saveProfile(profile); err != nil {
		return err
	}

	pm.profiles[profile.Name] = profile
	return nil
}

// Update updates an existing profile.
func (pm *ProfileManager) Update(profile *Profile) error {
	if _, exists := pm.profiles[profile.Name]; !exists {
		return fmt.Errorf("profile %q not found", profile.Name)
	}

	if err := pm.saveProfile(profile); err != nil {
		return err
	}

	pm.profiles[profile.Name] = profile
	return nil
}

// Delete removes a profile.
func (pm *ProfileManager) Delete(name string) error {
	if _, exists := pm.profiles[name]; !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	path := filepath.Join(pm.profilesDir, name+".yaml")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete profile file: %w", err)
	}

	delete(pm.profiles, name)
	return nil
}

// saveProfile saves a profile to disk.
func (pm *ProfileManager) saveProfile(profile *Profile) error {
	path := filepath.Join(pm.profilesDir, profile.Name+".yaml")

	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}

	return nil
}

// Resolve resolves a profile with inheritance.
func (pm *ProfileManager) Resolve(name string) (map[string]any, error) {
	profile, err := pm.Get(name)
	if err != nil {
		return nil, err
	}

	return pm.resolveWithVisited(profile, make(map[string]bool))
}

// resolveWithVisited resolves inheritance while detecting cycles.
func (pm *ProfileManager) resolveWithVisited(profile *Profile, visited map[string]bool) (map[string]any, error) {
	if visited[profile.Name] {
		return nil, fmt.Errorf("circular inheritance detected: %s", profile.Name)
	}
	visited[profile.Name] = true

	result := make(map[string]any)

	// First, resolve parent profiles
	for _, parentName := range profile.Inherits {
		parent, err := pm.Get(parentName)
		if err != nil {
			return nil, fmt.Errorf("parent %q: %w", parentName, err)
		}

		parentConfig, err := pm.resolveWithVisited(parent, visited)
		if err != nil {
			return nil, err
		}

		// Merge parent config
		for k, v := range parentConfig {
			result[k] = v
		}
	}

	// Then apply this profile's config (overrides parents)
	for k, v := range profile.Config {
		result[k] = v
	}

	return result, nil
}

// Clone creates a copy of an existing profile.
func (pm *ProfileManager) Clone(sourceName, newName string) (*Profile, error) {
	source, err := pm.Get(sourceName)
	if err != nil {
		return nil, err
	}

	if _, exists := pm.profiles[newName]; exists {
		return nil, fmt.Errorf("profile %q already exists", newName)
	}

	newProfile := &Profile{
		Name:        newName,
		Description: fmt.Sprintf("Clone of %s", sourceName),
		Inherits:    append([]string{}, source.Inherits...),
		Environment: source.Environment,
		Config:      make(map[string]any),
		Metadata:    source.Metadata,
	}

	for k, v := range source.Config {
		newProfile.Config[k] = v
	}

	if err := pm.Create(newProfile); err != nil {
		return nil, err
	}

	return newProfile, nil
}

// Export exports a profile to a file.
func (pm *ProfileManager) Export(name, path string) error {
	profile, err := pm.Get(name)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// Import imports a profile from a file.
func (pm *ProfileManager) Import(path string) (*Profile, error) {
	profile, err := pm.loadProfile(path)
	if err != nil {
		return nil, err
	}

	if err := pm.Create(profile); err != nil {
		return nil, err
	}

	return profile, nil
}

// DetectEnvironment returns the appropriate profile for the current environment.
func (pm *ProfileManager) DetectEnvironment() (*Profile, error) {
	// Check for environment indicator
	env := os.Getenv("AUTOMERGENT_ENV")
	if env == "" {
		env = os.Getenv("GO_ENV")
	}
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	if env == "" {
		env = os.Getenv("NODE_ENV")
	}

	// Try to find matching profile
	for _, profile := range pm.profiles {
		if profile.Environment == env {
			return profile, nil
		}
	}

	// Try common environment names
	commonEnvs := []string{"development", "dev", "production", "prod", "staging", "test"}
	for _, e := range commonEnvs {
		if strings.Contains(strings.ToLower(env), e) {
			if profile, ok := pm.profiles[e]; ok {
				return profile, nil
			}
		}
	}

	return nil, nil
}

// BuiltinProfiles returns predefined profiles.
func BuiltinProfiles() []*Profile {
	return []*Profile{
		{
			Name:        "dev",
			Description: "Development settings with verbose output and debugging enabled",
			Environment: "development",
			Config: map[string]any{
				"verbose":          true,
				"telemetry":        false,
				"log.level":        "debug",
				"security.sandbox": "off",
			},
			Metadata: ProfileMetadata{
				Tags: []string{"development", "debugging"},
			},
		},
		{
			Name:        "prod",
			Description: "Production settings optimized for performance and security",
			Environment: "production",
			Config: map[string]any{
				"verbose":          false,
				"quiet":            true,
				"telemetry":        true,
				"security.sandbox": "strict",
				"log.level":        "warn",
			},
			Metadata: ProfileMetadata{
				Tags: []string{"production", "secure"},
			},
		},
		{
			Name:        "ci",
			Description: "CI/CD pipeline settings with no interaction",
			Environment: "ci",
			Config: map[string]any{
				"noTui":       true,
				"noColor":     true,
				"noAnimation": true,
				"quiet":       true,
				"telemetry":   false,
			},
			Metadata: ProfileMetadata{
				Tags: []string{"ci", "automation"},
			},
		},
		{
			Name:        "minimal",
			Description: "Minimal resource usage for constrained environments",
			Config: map[string]any{
				"maxContextTokens":    32000,
				"maxAutoReadFileSize": 128 * 1024,
				"maxTreeFiles":        500,
				"maxTreeDepth":        5,
				"noAnimation":         true,
				"lsp.enabled":         false,
			},
			Metadata: ProfileMetadata{
				Tags: []string{"minimal", "low-resource"},
			},
		},
		{
			Name:        "secure",
			Description: "Maximum security settings",
			Config: map[string]any{
				"security.sandbox":                      "strict",
				"security.requireGitForAutoModes":       true,
				"zeroDataRetention":                     true,
				"telemetry":                             false,
				"tools.shell.confirmationRequired":      "always",
				"tools.filesystem.confirmationRequired": "always",
			},
			Metadata: ProfileMetadata{
				Tags: []string{"security", "privacy"},
			},
		},
		{
			Name:        "performance",
			Description: "Settings optimized for maximum speed",
			Config: map[string]any{
				"maxContextTokens":      200000,
				"autoCompressAt":        0.95,
				"compressionKeepRecent": 5,
				"lsp.enabled":           true,
				"diagnostics.enabled":   true,
			},
			Metadata: ProfileMetadata{
				Tags: []string{"performance", "speed"},
			},
		},
	}
}

// InstallBuiltinProfiles installs the builtin profiles if they don't exist.
func (pm *ProfileManager) InstallBuiltinProfiles() error {
	for _, profile := range BuiltinProfiles() {
		if _, exists := pm.profiles[profile.Name]; !exists {
			if err := pm.Create(profile); err != nil {
				return fmt.Errorf("install %s: %w", profile.Name, err)
			}
		}
	}
	return nil
}

// Search finds profiles matching a query.
func (pm *ProfileManager) Search(query string) []*Profile {
	query = strings.ToLower(query)
	var results []*Profile

	for _, profile := range pm.profiles {
		// Check name
		if strings.Contains(strings.ToLower(profile.Name), query) {
			results = append(results, profile)
			continue
		}

		// Check description
		if strings.Contains(strings.ToLower(profile.Description), query) {
			results = append(results, profile)
			continue
		}

		// Check tags
		for _, tag := range profile.Metadata.Tags {
			if strings.Contains(strings.ToLower(tag), query) {
				results = append(results, profile)
				break
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results
}
