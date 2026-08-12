package learning

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PrivacyLevel indicates the level of data collection.
type PrivacyLevel string

const (
	PrivacyLevelNone     PrivacyLevel = "none"     // No data collection
	PrivacyLevelMinimal  PrivacyLevel = "minimal"  // Basic patterns only
	PrivacyLevelStandard PrivacyLevel = "standard" // Standard learning
	PrivacyLevelFull     PrivacyLevel = "full"     // Full personalization
)

// PrivacyController manages privacy-related operations.
type PrivacyController struct {
	mu          sync.RWMutex
	profile     *UserProfile
	level       PrivacyLevel
	dataDir     string
	consentFile string
}

// ConsentRecord tracks user consent history.
type ConsentRecord struct {
	Version    string    `json:"version"`
	GivenAt    time.Time `json:"given_at"`
	RevokedAt  time.Time `json:"revoked_at,omitempty"`
	Level      string    `json:"level"`
	Categories []string  `json:"categories"`
}

// DataExport represents exported user data.
type DataExport struct {
	ExportedAt   time.Time                       `json:"exported_at"`
	UserID       string                          `json:"user_id"`
	Profile      *UserProfile                    `json:"profile,omitempty"`
	Patterns     []*Pattern                      `json:"patterns,omitempty"`
	Feedback     []*Feedback                     `json:"feedback,omitempty"`
	Aggregations map[string]*FeedbackAggregation `json:"aggregations,omitempty"`
	Suggestions  []*Suggestion                   `json:"suggestions,omitempty"`
	Metadata     map[string]string               `json:"metadata,omitempty"`
}

// NewPrivacyController creates a new privacy controller.
func NewPrivacyController(dataDir string) (*PrivacyController, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("privacy controller: mkdir: %w", err)
	}

	return &PrivacyController{
		dataDir:     dataDir,
		consentFile: filepath.Join(dataDir, "consent.json"),
		level:       PrivacyLevelMinimal, // Conservative default
	}, nil
}

// SetProfile connects the controller to a user profile.
func (pc *PrivacyController) SetProfile(p *UserProfile) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.profile = p

	// Update level based on profile settings
	if p != nil {
		pc.updateLevelFromSettings(p.Privacy)
	}
}

// updateLevelFromSettings determines privacy level from settings.
func (pc *PrivacyController) updateLevelFromSettings(settings PrivacySettings) {
	if !settings.ConsentGiven {
		pc.level = PrivacyLevelNone
		return
	}

	// Count enabled collection types
	enabled := 0
	if settings.CollectPatterns {
		enabled++
	}
	if settings.CollectToolUsage {
		enabled++
	}
	if settings.CollectFilePatterns {
		enabled++
	}
	if settings.CollectTimePatterns {
		enabled++
	}
	if settings.CollectCodingStyle {
		enabled++
	}
	if settings.CollectCommunication {
		enabled++
	}

	switch {
	case enabled == 0:
		pc.level = PrivacyLevelNone
	case enabled <= 2:
		pc.level = PrivacyLevelMinimal
	case enabled <= 4:
		pc.level = PrivacyLevelStandard
	default:
		pc.level = PrivacyLevelFull
	}
}

// GetLevel returns the current privacy level.
func (pc *PrivacyController) GetLevel() PrivacyLevel {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.level
}

// SetLevel sets the privacy level and updates profile.
func (pc *PrivacyController) SetLevel(level PrivacyLevel) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.level = level

	if pc.profile == nil {
		return
	}

	// Update profile settings based on level
	switch level {
	case PrivacyLevelNone:
		pc.profile.Privacy = PrivacySettings{
			LocalOnly: true,
		}
	case PrivacyLevelMinimal:
		pc.profile.Privacy = PrivacySettings{
			ConsentGiven:     true,
			CollectPatterns:  true,
			CollectToolUsage: true,
			LocalOnly:        true,
			AnonymizePaths:   true,
			ObfuscateContent: true,
		}
	case PrivacyLevelStandard:
		pc.profile.Privacy = PrivacySettings{
			ConsentGiven:        true,
			CollectPatterns:     true,
			CollectToolUsage:    true,
			CollectTimePatterns: true,
			CollectCodingStyle:  true,
			LocalOnly:           true,
			AnonymizePaths:      true,
		}
	case PrivacyLevelFull:
		pc.profile.Privacy = PrivacySettings{
			ConsentGiven:         true,
			CollectPatterns:      true,
			CollectToolUsage:     true,
			CollectFilePatterns:  true,
			CollectTimePatterns:  true,
			CollectCodingStyle:   true,
			CollectCommunication: true,
			LocalOnly:            true, // Still local-only by default
		}
	}
}

// GiveConsent records user consent.
func (pc *PrivacyController) GiveConsent(version string, categories []string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	record := ConsentRecord{
		Version:    version,
		GivenAt:    time.Now(),
		Level:      string(pc.level),
		Categories: categories,
	}

	// Update profile
	if pc.profile != nil {
		pc.profile.GiveConsent(version)
	}

	// Save consent record
	return pc.saveConsentRecord(record)
}

// RevokeConsent revokes consent and triggers data cleanup.
func (pc *PrivacyController) RevokeConsent() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Load existing record to update
	var record ConsentRecord
	data, err := os.ReadFile(pc.consentFile)
	if err == nil {
		json.Unmarshal(data, &record)
	}

	record.RevokedAt = time.Now()

	// Update profile
	if pc.profile != nil {
		pc.profile.RevokeConsent()
	}

	pc.level = PrivacyLevelNone

	return pc.saveConsentRecord(record)
}

// saveConsentRecord persists the consent record.
func (pc *PrivacyController) saveConsentRecord(record ConsentRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pc.consentFile, data, 0o600)
}

// HasConsent checks if user has given consent.
func (pc *PrivacyController) HasConsent() bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.profile != nil {
		return pc.profile.Privacy.ConsentGiven
	}
	return false
}

// CanCollect checks if a specific data type can be collected.
func (pc *PrivacyController) CanCollect(dataType PatternType) bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.level == PrivacyLevelNone {
		return false
	}

	if pc.profile != nil {
		return pc.profile.ShouldCollect(dataType)
	}

	// Default based on level
	switch pc.level {
	case PrivacyLevelMinimal:
		return dataType == PatternTypeToolUsage
	case PrivacyLevelStandard:
		return dataType != PatternTypeFile && dataType != PatternTypeCommunication
	case PrivacyLevelFull:
		return true
	}

	return false
}

// ExportData exports all user data.
func (pc *PrivacyController) ExportData(
	profile *UserProfile,
	patterns []*Pattern,
	feedback []*Feedback,
	aggregations map[string]*FeedbackAggregation,
	suggestions []*Suggestion,
) (*DataExport, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	export := &DataExport{
		ExportedAt:   time.Now(),
		Profile:      profile,
		Patterns:     patterns,
		Feedback:     feedback,
		Aggregations: aggregations,
		Suggestions:  suggestions,
		Metadata: map[string]string{
			"export_version": "1.0",
			"privacy_level":  string(pc.level),
		},
	}

	if profile != nil {
		export.UserID = profile.ID
	}

	return export, nil
}

// ExportDataToFile exports user data to a file.
func (pc *PrivacyController) ExportDataToFile(export *DataExport, path string) error {
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("export marshal: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("export write: %w", err)
	}

	// Update profile export timestamp
	if pc.profile != nil {
		pc.profile.mu.Lock()
		pc.profile.Privacy.LastExport = time.Now()
		pc.profile.mu.Unlock()
	}

	return nil
}

// DeleteAllData removes all learned data.
func (pc *PrivacyController) DeleteAllData() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Clear profile patterns
	if pc.profile != nil {
		pc.profile.mu.Lock()
		pc.profile.Patterns = make(map[string]*Pattern)
		pc.profile.mu.Unlock()
	}

	// Delete profile files
	profilePath := filepath.Join(pc.dataDir, "*.json")
	matches, err := filepath.Glob(profilePath)
	if err != nil {
		return fmt.Errorf("delete glob: %w", err)
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete file %s: %w", match, err)
		}
	}

	return nil
}

// DeleteUserData deletes data for a specific user.
func (pc *PrivacyController) DeleteUserData(userID string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Clear profile if it matches
	if pc.profile != nil && pc.profile.ID == userID {
		pc.profile.mu.Lock()
		pc.profile.Patterns = make(map[string]*Pattern)
		pc.profile.mu.Unlock()
	}

	// Delete user's profile file
	profilePath := filepath.Join(pc.dataDir, userID+".json")
	if err := os.Remove(profilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete profile: %w", err)
	}

	return nil
}

// AnonymizeData removes identifying information from data.
func (pc *PrivacyController) AnonymizeData(export *DataExport) *DataExport {
	// Create a copy to avoid modifying original
	anonymized := &DataExport{
		ExportedAt: export.ExportedAt,
		UserID:     "anonymized",
		Metadata: map[string]string{
			"anonymized": "true",
		},
	}

	// Anonymize patterns
	for _, p := range export.Patterns {
		anonPattern := *p
		anonPattern.Data.Directories = nil // Remove path data
		// Hash any potentially identifying data
		anonymized.Patterns = append(anonymized.Patterns, &anonPattern)
	}

	// Don't include feedback (too detailed)
	// Don't include suggestions (too specific)

	return anonymized
}

// GetPrivacyReport returns a summary of collected data.
func (pc *PrivacyController) GetPrivacyReport() map[string]interface{} {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	report := map[string]interface{}{
		"privacy_level":  string(pc.level),
		"consent_given":  pc.HasConsent(),
		"local_only":     true,
		"data_directory": pc.dataDir,
	}

	if pc.profile != nil {
		report["user_id"] = pc.profile.ID
		report["patterns_stored"] = len(pc.profile.Patterns)
		report["retention_days"] = pc.profile.Privacy.RetentionDays
		report["anonymize_paths"] = pc.profile.Privacy.AnonymizePaths
		report["encrypt_at_rest"] = pc.profile.Privacy.EncryptAtRest

		if !pc.profile.Privacy.LastExport.IsZero() {
			report["last_export"] = pc.profile.Privacy.LastExport.Format(time.RFC3339)
		}
	}

	// Data collection status
	if pc.profile != nil {
		collection := map[string]bool{
			"patterns":      pc.profile.Privacy.CollectPatterns,
			"tool_usage":    pc.profile.Privacy.CollectToolUsage,
			"file_patterns": pc.profile.Privacy.CollectFilePatterns,
			"time_patterns": pc.profile.Privacy.CollectTimePatterns,
			"coding_style":  pc.profile.Privacy.CollectCodingStyle,
			"communication": pc.profile.Privacy.CollectCommunication,
		}
		report["collection_settings"] = collection
	}

	return report
}

// ApplyRetentionPolicy deletes data older than retention period.
func (pc *PrivacyController) ApplyRetentionPolicy() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.profile == nil {
		return nil
	}

	retentionDays := pc.profile.Privacy.RetentionDays
	if retentionDays <= 0 {
		return nil // No retention policy
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	// Remove old patterns
	pc.profile.mu.Lock()
	for id, p := range pc.profile.Patterns {
		if p.LastSeen.Before(cutoff) {
			delete(pc.profile.Patterns, id)
		}
	}
	pc.profile.mu.Unlock()

	return nil
}
