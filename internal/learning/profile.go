package learning

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UserProfile represents a user's learned preferences and patterns.
type UserProfile struct {
	mu sync.RWMutex

	UserID    string              `json:"user_id"`
	ID        string              `json:"id"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
	Version   int                 `json:"version"`
	Patterns  map[string]*Pattern `json:"patterns,omitempty"`

	// Learned preferences
	Preferences UserPreferences `json:"preferences"`

	// Session statistics
	Stats ProfileStats `json:"stats"`

	// Privacy settings
	Privacy PrivacySettings `json:"privacy"`
}

// UserPreferences holds learned user preferences.
type UserPreferences struct {
	// Coding preferences
	PreferredLanguages   []string `json:"preferred_languages,omitempty"`
	PreferredFrameworks  []string `json:"preferred_frameworks,omitempty"`
	PreferredEditor      string   `json:"preferred_editor,omitempty"`
	PreferredIndentation string   `json:"preferred_indentation,omitempty"` // "tabs" or "spaces:N"

	// Tool preferences
	FavoriteTools    []string          `json:"favorite_tools,omitempty"`
	ToolAliases      map[string]string `json:"tool_aliases,omitempty"`
	DefaultToolArgs  map[string]string `json:"default_tool_args,omitempty"`
	DisabledFeatures []string          `json:"disabled_features,omitempty"`

	// Communication preferences
	ResponseVerbosity string `json:"response_verbosity,omitempty"` // "brief", "moderate", "detailed"
	TechnicalLevel    string `json:"technical_level,omitempty"`    // "beginner", "intermediate", "expert"
	ShowExplanations  bool   `json:"show_explanations,omitempty"`
	PreferCodeBlocks  bool   `json:"prefer_code_blocks,omitempty"`

	// Workflow preferences
	AutoConfirm      []string `json:"auto_confirm,omitempty"`      // actions to auto-confirm
	SkipWarnings     []string `json:"skip_warnings,omitempty"`     // warnings to skip
	PreferredMode    string   `json:"preferred_mode,omitempty"`    // "auto", "manual", etc.
	DefaultBranch    string   `json:"default_branch,omitempty"`    // git branch preference
	WorkingDirectory string   `json:"working_directory,omitempty"` // preferred cwd
	PreferredShell   string   `json:"preferred_shell,omitempty"`   // bash, zsh, etc.
	PinnedContexts   []string `json:"pinned_contexts,omitempty"`   // always-included context

	// UI preferences
	Theme            string `json:"theme,omitempty"`
	CompactMode      bool   `json:"compact_mode,omitempty"`
	ShowTimestamps   bool   `json:"show_timestamps,omitempty"`
	HighlightChanges bool   `json:"highlight_changes,omitempty"`
}

// ProfileStats holds aggregate statistics.
type ProfileStats struct {
	TotalSessions     int       `json:"total_sessions"`
	TotalMessages     int       `json:"total_messages"`
	TotalTokensUsed   int64     `json:"total_tokens_used"`
	TotalToolCalls    int       `json:"total_tool_calls"`
	TotalFilesEdited  int       `json:"total_files_edited"`
	TotalErrors       int       `json:"total_errors"`
	SuccessfulTasks   int       `json:"successful_tasks"`
	FirstSessionAt    time.Time `json:"first_session_at"`
	LastSessionAt     time.Time `json:"last_session_at"`
	AvgSessionLength  float64   `json:"avg_session_length_min"`
	MostActiveHour    int       `json:"most_active_hour"`
	MostActiveWeekday int       `json:"most_active_weekday"`
}

// PrivacySettings controls what data is collected and stored.
type PrivacySettings struct {
	// Data collection
	CollectPatterns      bool `json:"collect_patterns"`      // learn from usage patterns
	CollectToolUsage     bool `json:"collect_tool_usage"`    // track tool preferences
	CollectFilePatterns  bool `json:"collect_file_patterns"` // track file access patterns
	CollectTimePatterns  bool `json:"collect_time_patterns"` // track activity times
	CollectCodingStyle   bool `json:"collect_coding_style"`  // analyze code for style
	CollectCommunication bool `json:"collect_communication"` // analyze message style

	// Data storage
	LocalOnly     bool `json:"local_only"`      // never sync to cloud
	EncryptAtRest bool `json:"encrypt_at_rest"` // encrypt stored data
	RetentionDays int  `json:"retention_days"`  // auto-delete after N days (0 = forever)

	// Anonymization
	AnonymizePaths   bool `json:"anonymize_paths"`   // hash file paths
	StripUsernames   bool `json:"strip_usernames"`   // remove usernames from patterns
	ObfuscateContent bool `json:"obfuscate_content"` // don't store actual content

	// User consent
	ConsentGiven   bool      `json:"consent_given"`
	ConsentDate    time.Time `json:"consent_date,omitempty"`
	ConsentVersion string    `json:"consent_version,omitempty"`

	// Export/delete
	LastExport    time.Time `json:"last_export,omitempty"`
	DataRequested bool      `json:"data_requested"`
}

// NewUserProfile creates a new user profile with default settings.
func NewUserProfile(id string) *UserProfile {
	now := time.Now()
	return &UserProfile{
		UserID:    id,
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   1,
		Patterns:  make(map[string]*Pattern),
		Preferences: UserPreferences{
			ShowExplanations: true,
			PreferCodeBlocks: true,
		},
		Stats: ProfileStats{
			FirstSessionAt: now,
		},
		Privacy: DefaultPrivacySettings(),
	}
}

// DefaultPrivacySettings returns conservative privacy defaults.
func DefaultPrivacySettings() PrivacySettings {
	return PrivacySettings{
		// Minimal collection by default
		CollectPatterns:      true,
		CollectToolUsage:     true,
		CollectFilePatterns:  false, // opt-in
		CollectTimePatterns:  true,
		CollectCodingStyle:   true,
		CollectCommunication: false, // opt-in

		// Strong privacy defaults
		LocalOnly:     true,  // never sync by default
		EncryptAtRest: false, // optional
		RetentionDays: 90,    // 3 months

		// Anonymization on
		AnonymizePaths:   true,
		StripUsernames:   true,
		ObfuscateContent: true,

		ConsentGiven: false,
	}
}

// UpdatePattern adds or updates a pattern in the profile.
func (p *UserProfile) UpdatePattern(pattern *Pattern) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Patterns[pattern.ID] = pattern
	p.UpdatedAt = time.Now()
}

// GetPattern retrieves a pattern by ID.
func (p *UserProfile) GetPattern(id string) *Pattern {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.Patterns[id]
}

// RecordSession updates stats for a new session.
func (p *UserProfile) RecordSession(messages int, tokens int64, toolCalls int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Stats.TotalSessions++
	p.Stats.TotalMessages += messages
	p.Stats.TotalTokensUsed += tokens
	p.Stats.TotalToolCalls += toolCalls
	p.Stats.LastSessionAt = time.Now()
	p.UpdatedAt = time.Now()
}

// RecordError increments error count.
func (p *UserProfile) RecordError() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Stats.TotalErrors++
	p.UpdatedAt = time.Now()
}

// RecordSuccess increments successful task count.
func (p *UserProfile) RecordSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Stats.SuccessfulTasks++
	p.UpdatedAt = time.Now()
}

// SetPreference updates a specific preference.
func (p *UserProfile) SetPreference(key string, value interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch key {
	case "preferred_languages":
		if v, ok := value.([]string); ok {
			p.Preferences.PreferredLanguages = v
		}
	case "response_verbosity":
		if v, ok := value.(string); ok {
			p.Preferences.ResponseVerbosity = v
		}
	case "technical_level":
		if v, ok := value.(string); ok {
			p.Preferences.TechnicalLevel = v
		}
	case "theme":
		if v, ok := value.(string); ok {
			p.Preferences.Theme = v
		}
	case "compact_mode":
		if v, ok := value.(bool); ok {
			p.Preferences.CompactMode = v
		}
	case "show_explanations":
		if v, ok := value.(bool); ok {
			p.Preferences.ShowExplanations = v
		}
	default:
		return fmt.Errorf("unknown preference: %s", key)
	}

	p.UpdatedAt = time.Now()
	return nil
}

// GetPrivacySettings returns current privacy settings.
func (p *UserProfile) GetPrivacySettings() PrivacySettings {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.Privacy
}

// UpdatePrivacy updates privacy settings.
func (p *UserProfile) UpdatePrivacy(settings PrivacySettings) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Privacy = settings
	p.UpdatedAt = time.Now()
}

// GiveConsent records user consent for data collection.
func (p *UserProfile) GiveConsent(version string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Privacy.ConsentGiven = true
	p.Privacy.ConsentDate = time.Now()
	p.Privacy.ConsentVersion = version
	p.UpdatedAt = time.Now()
}

// RevokeConsent revokes consent and clears collected data.
func (p *UserProfile) RevokeConsent() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Privacy.ConsentGiven = false
	p.Patterns = make(map[string]*Pattern) // Clear all patterns
	p.UpdatedAt = time.Now()
}

// ShouldCollect checks if a specific type of data should be collected.
func (p *UserProfile) ShouldCollect(dataType PatternType) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.Privacy.ConsentGiven {
		return false
	}

	switch dataType {
	case PatternTypeToolUsage:
		return p.Privacy.CollectToolUsage
	case PatternTypeFile:
		return p.Privacy.CollectFilePatterns
	case PatternTypeTime:
		return p.Privacy.CollectTimePatterns
	case PatternTypeCodingStyle:
		return p.Privacy.CollectCodingStyle
	case PatternTypeCommunication:
		return p.Privacy.CollectCommunication
	default:
		return p.Privacy.CollectPatterns
	}
}

// ProfileStorage handles persistence of user profiles.
type ProfileStorage struct {
	dir string
}

// NewProfileStorage creates a new profile storage.
func NewProfileStorage(dir string) (*ProfileStorage, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("profile storage: mkdir: %w", err)
	}
	return &ProfileStorage{dir: dir}, nil
}

// Save writes a profile to disk atomically.
func (s *ProfileStorage) Save(profile *UserProfile) error {
	profile.mu.RLock()
	data, err := json.MarshalIndent(profile, "", "  ")
	profile.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("profile save: marshal: %w", err)
	}

	path := filepath.Join(s.dir, profile.ID+".json")
	return atomicWriteFile(path, data, 0o600)
}

// Load reads a profile from disk.
func (s *ProfileStorage) Load(id string) (*UserProfile, error) {
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Not found is not an error
		}
		return nil, err
	}

	var profile UserProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

// Delete removes a profile from disk.
func (s *ProfileStorage) Delete(id string) error {
	return os.Remove(filepath.Join(s.dir, id+".json"))
}

// Export exports profile data in a portable format.
func (s *ProfileStorage) Export(id string) ([]byte, error) {
	profile, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("profile not found: %s", id)
	}

	profile.mu.Lock()
	profile.Privacy.LastExport = time.Now()
	profile.mu.Unlock()

	// Save with updated export time
	if err := s.Save(profile); err != nil {
		return nil, err
	}

	return json.MarshalIndent(profile, "", "  ")
}

// atomicWriteFile writes data atomically.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".automergent-profile-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	return os.Rename(tmpPath, path)
}
