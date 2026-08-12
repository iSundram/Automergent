package installer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// UpdateConfig holds update preferences
type UpdateConfig struct {
	AutoCheck     bool      `json:"auto_check"`
	AutoUpdate    bool      `json:"auto_update"`
	CheckInterval int       `json:"check_interval_hours"` // Hours between checks
	LastCheck     time.Time `json:"last_check"`
	Channel       string    `json:"channel"` // stable, beta, nightly
}

// UpdateInfo represents available update information
type UpdateInfo struct {
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
	ReleaseNotes    string    `json:"release_notes,omitempty"`
	ReleaseURL      string    `json:"release_url,omitempty"`
	PublishedAt     time.Time `json:"published_at,omitempty"`
	DownloadURL     string    `json:"download_url,omitempty"`
	Size            int64     `json:"size,omitempty"`
}

// Changelog represents a version changelog entry
type Changelog struct {
	Version    string    `json:"version"`
	Date       time.Time `json:"date"`
	Changes    []string  `json:"changes"`
	Breaking   []string  `json:"breaking,omitempty"`
	Deprecated []string  `json:"deprecated,omitempty"`
}

// UpdateResult holds the result of an update operation
type UpdateResult struct {
	Success       bool   `json:"success"`
	FromVersion   string `json:"from_version"`
	ToVersion     string `json:"to_version"`
	BackupPath    string `json:"backup_path,omitempty"`
	Error         string `json:"error,omitempty"`
	RestartNeeded bool   `json:"restart_needed"`
}

// RollbackInfo holds information for rollback operations
type RollbackInfo struct {
	Version    string    `json:"version"`
	BackupPath string    `json:"backup_path"`
	Timestamp  time.Time `json:"timestamp"`
}

// DefaultUpdateConfig returns default update configuration
func DefaultUpdateConfig() *UpdateConfig {
	return &UpdateConfig{
		AutoCheck:     true,
		AutoUpdate:    false,
		CheckInterval: 24, // Check once per day
		Channel:       "stable",
	}
}

// GetUpdateConfigPath returns the path to update config file
func GetUpdateConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".automergent", "update-config.json"), nil
}

// LoadUpdateConfig loads update configuration
func LoadUpdateConfig() (*UpdateConfig, error) {
	configPath, err := GetUpdateConfigPath()
	if err != nil {
		return DefaultUpdateConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultUpdateConfig(), nil
		}
		return nil, err
	}

	config := &UpdateConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return DefaultUpdateConfig(), nil
	}

	return config, nil
}

// SaveUpdateConfig saves update configuration
func SaveUpdateConfig(config *UpdateConfig) error {
	configPath, err := GetUpdateConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// CheckForUpdates checks if a new version is available
func CheckForUpdates(currentVersion string) (*UpdateInfo, error) {
	latestVersion, err := GetLatestVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}

	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
	}

	// Compare versions (simple string comparison for semver)
	info.UpdateAvailable = compareVersions(currentVersion, latestVersion) < 0

	if info.UpdateAvailable {
		// Fetch release info
		release, err := fetchReleaseInfo("v" + latestVersion)
		if err == nil {
			info.ReleaseNotes = release.Body
			info.ReleaseURL = release.HTMLURL
			info.PublishedAt = release.PublishedAt

			// Find download URL for current platform
			for _, asset := range release.Assets {
				if matchesCurrentPlatform(asset.Name) {
					info.DownloadURL = asset.BrowserDownloadURL
					info.Size = asset.Size
					break
				}
			}
		}
	}

	// Update last check time
	config, _ := LoadUpdateConfig()
	config.LastCheck = time.Now()
	_ = SaveUpdateConfig(config)

	return info, nil
}

// GithubRelease represents a GitHub release
type GithubRelease struct {
	TagName     string    `json:"tag_name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// fetchReleaseInfo fetches release information from GitHub
func fetchReleaseInfo(tag string) (*GithubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", Repo, tag)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch release: %s", resp.Status)
	}

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// matchesCurrentPlatform checks if asset name matches current OS/arch
func matchesCurrentPlatform(name string) bool {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	suffix := fmt.Sprintf("%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
	return strings.HasPrefix(name, "automergent_") && strings.HasSuffix(name, suffix)
}

// compareVersions compares two semver strings
// Returns -1 if v1 < v2, 0 if equal, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Strip 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &n2)
		}

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	return 0
}

// PerformUpdate downloads and installs the latest version
func PerformUpdate(info *UpdateInfo, progressChan chan float64) (*UpdateResult, error) {
	result := &UpdateResult{
		FromVersion: info.CurrentVersion,
		ToVersion:   info.LatestVersion,
	}

	// Find current binary location
	currentBinary, err := os.Executable()
	if err != nil {
		result.Error = fmt.Sprintf("failed to locate current binary: %v", err)
		return result, err
	}

	// Create backup
	backupPath, err := createBackup(currentBinary, info.CurrentVersion)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create backup: %v", err)
		return result, err
	}
	result.BackupPath = backupPath

	// Download new version
	sysInfo, err := GetSystemInfo()
	if err != nil {
		result.Error = fmt.Sprintf("failed to get system info: %v", err)
		return result, err
	}

	archivePath, err := DownloadBinary(info.LatestVersion, sysInfo, progressChan)
	if err != nil {
		result.Error = fmt.Sprintf("failed to download update: %v", err)
		return result, err
	}
	defer os.Remove(archivePath)

	// Extract to temp location
	tempDir, err := os.MkdirTemp("", "automergent-update-*")
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp dir: %v", err)
		return result, err
	}
	defer os.RemoveAll(tempDir)

	if err := ExtractBinary(archivePath, tempDir); err != nil {
		result.Error = fmt.Sprintf("failed to extract update: %v", err)
		return result, err
	}

	// Replace binary
	newBinary := filepath.Join(tempDir, "automergent")
	if runtime.GOOS == "windows" {
		newBinary += ".exe"
	}

	// On Windows, we need to rename the current binary first
	if runtime.GOOS == "windows" {
		oldPath := currentBinary + ".old"
		os.Remove(oldPath) // Remove any existing .old file
		if err := os.Rename(currentBinary, oldPath); err != nil {
			result.Error = fmt.Sprintf("failed to rename current binary: %v", err)
			return result, err
		}
	}

	// Copy new binary
	if err := copyFile(newBinary, currentBinary); err != nil {
		// Try to restore on failure
		if runtime.GOOS == "windows" {
			os.Rename(currentBinary+".old", currentBinary)
		}
		result.Error = fmt.Sprintf("failed to install update: %v", err)
		return result, err
	}

	// Make executable (Unix only)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(currentBinary, 0755); err != nil {
			result.Error = fmt.Sprintf("failed to set permissions: %v", err)
			return result, err
		}
	}

	// Save rollback info
	if err := saveRollbackInfo(info.CurrentVersion, backupPath); err != nil {
		// Non-fatal
	}

	result.Success = true
	result.RestartNeeded = true
	return result, nil
}

// createBackup creates a backup of the current binary
func createBackup(binaryPath, version string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	backupDir := filepath.Join(home, ".automergent", "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("automergent-%s-%s", version, timestamp)
	if runtime.GOOS == "windows" {
		backupName += ".exe"
	}

	backupPath := filepath.Join(backupDir, backupName)
	return backupPath, copyFile(binaryPath, backupPath)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// saveRollbackInfo saves rollback information
func saveRollbackInfo(version, backupPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	rollbackFile := filepath.Join(home, ".automergent", "rollback.json")
	info := &RollbackInfo{
		Version:    version,
		BackupPath: backupPath,
		Timestamp:  time.Now(),
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(rollbackFile, data, 0644)
}

// GetRollbackInfo retrieves rollback information
func GetRollbackInfo() (*RollbackInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	rollbackFile := filepath.Join(home, ".automergent", "rollback.json")
	data, err := os.ReadFile(rollbackFile)
	if err != nil {
		return nil, err
	}

	info := &RollbackInfo{}
	if err := json.Unmarshal(data, info); err != nil {
		return nil, err
	}

	return info, nil
}

// PerformRollback restores the previous version
func PerformRollback() (*UpdateResult, error) {
	info, err := GetRollbackInfo()
	if err != nil {
		return nil, fmt.Errorf("no rollback info available: %w", err)
	}

	// Check backup exists
	if _, err := os.Stat(info.BackupPath); err != nil {
		return nil, fmt.Errorf("backup file not found: %s", info.BackupPath)
	}

	// Get current binary path
	currentBinary, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to locate current binary: %w", err)
	}

	// Get current version for result
	currentVersion := ""
	cmd := exec.Command(currentBinary, "--version")
	if output, err := cmd.Output(); err == nil {
		currentVersion = strings.TrimSpace(string(output))
	}

	result := &UpdateResult{
		FromVersion: currentVersion,
		ToVersion:   info.Version,
	}

	// Create backup of current version (in case user wants to undo rollback)
	backupPath, err := createBackup(currentBinary, currentVersion)
	if err == nil {
		result.BackupPath = backupPath
	}

	// Restore backup
	if runtime.GOOS == "windows" {
		oldPath := currentBinary + ".old"
		os.Remove(oldPath)
		if err := os.Rename(currentBinary, oldPath); err != nil {
			result.Error = fmt.Sprintf("failed to rename current binary: %v", err)
			return result, err
		}
	}

	if err := copyFile(info.BackupPath, currentBinary); err != nil {
		if runtime.GOOS == "windows" {
			os.Rename(currentBinary+".old", currentBinary)
		}
		result.Error = fmt.Sprintf("failed to restore backup: %v", err)
		return result, err
	}

	if runtime.GOOS != "windows" {
		os.Chmod(currentBinary, 0755)
	}

	result.Success = true
	result.RestartNeeded = true
	return result, nil
}

// ListBackups returns available backup versions
func ListBackups() ([]RollbackInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	backupDir := filepath.Join(home, ".automergent", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RollbackInfo{}, nil
		}
		return nil, err
	}

	var backups []RollbackInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Parse version from filename: automergent-VERSION-TIMESTAMP
		name := entry.Name()
		if !strings.HasPrefix(name, "automergent-") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Extract version (between first and second dash)
		parts := strings.SplitN(strings.TrimSuffix(name, filepath.Ext(name)), "-", 3)
		version := ""
		if len(parts) >= 2 {
			version = parts[1]
		}

		backups = append(backups, RollbackInfo{
			Version:    version,
			BackupPath: filepath.Join(backupDir, name),
			Timestamp:  info.ModTime(),
		})
	}

	return backups, nil
}

// CleanupOldBackups removes backups older than specified days
func CleanupOldBackups(daysToKeep int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	backupDir := filepath.Join(home, ".automergent", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -daysToKeep)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(backupDir, entry.Name()))
		}
	}

	return nil
}

// ShouldCheckForUpdates determines if we should check for updates
func ShouldCheckForUpdates() bool {
	config, err := LoadUpdateConfig()
	if err != nil || !config.AutoCheck {
		return false
	}

	// Check if enough time has passed since last check
	intervalDuration := time.Duration(config.CheckInterval) * time.Hour
	return time.Since(config.LastCheck) >= intervalDuration
}

// GetChangelog fetches the changelog for a specific version or range
func GetChangelog(fromVersion, toVersion string) ([]Changelog, error) {
	// Fetch releases from GitHub
	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases", Repo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var releases []GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	var changelogs []Changelog
	collecting := fromVersion == ""

	for _, release := range releases {
		version := strings.TrimPrefix(release.TagName, "v")

		// Start collecting if we've reached fromVersion
		if !collecting && version == fromVersion {
			collecting = true
			continue // Skip fromVersion itself
		}

		if collecting {
			changelog := Changelog{
				Version: version,
				Date:    release.PublishedAt,
			}

			// Parse release body for changes
			lines := strings.Split(release.Body, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
					change := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")

					if strings.Contains(strings.ToLower(line), "breaking") {
						changelog.Breaking = append(changelog.Breaking, change)
					} else if strings.Contains(strings.ToLower(line), "deprecated") {
						changelog.Deprecated = append(changelog.Deprecated, change)
					} else {
						changelog.Changes = append(changelog.Changes, change)
					}
				}
			}

			changelogs = append(changelogs, changelog)

			// Stop at toVersion
			if version == toVersion {
				break
			}
		}
	}

	return changelogs, nil
}
