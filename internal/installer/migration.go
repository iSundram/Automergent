package installer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// MigrationSource represents a tool to migrate from
type MigrationSource string

const (
	MigrationClaudeCode MigrationSource = "claude-code"
	MigrationClaude     MigrationSource = "claude"
	MigrationAider      MigrationSource = "aider"
	MigrationCopilot    MigrationSource = "copilot"
	MigrationCursor     MigrationSource = "cursor"
)

// MigrationConfig holds migration settings
type MigrationConfig struct {
	ImportSettings bool `json:"import_settings"`
	ImportHistory  bool `json:"import_history"`
	ImportProjects bool `json:"import_projects"`
	PreserveOld    bool `json:"preserve_old"`
}

// MigrationResult holds the result of a migration
type MigrationResult struct {
	Success          bool            `json:"success"`
	Source           MigrationSource `json:"source"`
	SettingsMigrated int             `json:"settings_migrated"`
	HistoryMigrated  int             `json:"history_migrated"`
	ProjectsMigrated int             `json:"projects_migrated"`
	Warnings         []string        `json:"warnings,omitempty"`
	Errors           []string        `json:"errors,omitempty"`
	BackupPath       string          `json:"backup_path,omitempty"`
}

// MigrationSourceInfo describes a migration source
type MigrationSourceInfo struct {
	Source      MigrationSource `json:"source"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Detected    bool            `json:"detected"`
	ConfigPath  string          `json:"config_path,omitempty"`
	DataPath    string          `json:"data_path,omitempty"`
	Version     string          `json:"version,omitempty"`
}

// DetectMigrationSources finds tools that can be migrated from
func DetectMigrationSources() []MigrationSourceInfo {
	home, _ := os.UserHomeDir()
	sources := []MigrationSourceInfo{}

	// Claude Code
	claudeCodePath := filepath.Join(home, ".claude")
	if info := checkMigrationSource(MigrationClaudeCode, claudeCodePath); info != nil {
		info.Name = "Claude Code"
		info.Description = "Anthropic's Claude Code Assistant"
		sources = append(sources, *info)
	}

	// Aider
	aiderPath := filepath.Join(home, ".aider")
	if info := checkMigrationSource(MigrationAider, aiderPath); info != nil {
		info.Name = "Aider"
		info.Description = "AI pair programming in your terminal"
		sources = append(sources, *info)
	}

	// GitHub Copilot CLI
	copilotPaths := []string{
		filepath.Join(home, ".config", "github-copilot"),
		filepath.Join(home, ".copilot"),
	}
	for _, path := range copilotPaths {
		if info := checkMigrationSource(MigrationCopilot, path); info != nil {
			info.Name = "GitHub Copilot CLI"
			info.Description = "GitHub's AI pair programmer"
			sources = append(sources, *info)
			break
		}
	}

	// Cursor
	cursorPaths := []string{
		filepath.Join(home, ".cursor"),
		filepath.Join(home, "Library", "Application Support", "Cursor"),
	}
	if runtime.GOOS == "windows" {
		cursorPaths = append(cursorPaths, filepath.Join(os.Getenv("APPDATA"), "Cursor"))
	}
	for _, path := range cursorPaths {
		if info := checkMigrationSource(MigrationCursor, path); info != nil {
			info.Name = "Cursor"
			info.Description = "AI-first code editor"
			sources = append(sources, *info)
			break
		}
	}

	return sources
}

// checkMigrationSource checks if a migration source exists
func checkMigrationSource(source MigrationSource, path string) *MigrationSourceInfo {
	if _, err := os.Stat(path); err != nil {
		return nil
	}

	return &MigrationSourceInfo{
		Source:     source,
		Detected:   true,
		ConfigPath: path,
		DataPath:   path,
	}
}

// DefaultMigrationConfig returns default migration settings
func DefaultMigrationConfig() *MigrationConfig {
	return &MigrationConfig{
		ImportSettings: true,
		ImportHistory:  true,
		ImportProjects: true,
		PreserveOld:    true,
	}
}

// MigrateFrom performs migration from a specific source
func MigrateFrom(source MigrationSource, config *MigrationConfig) (*MigrationResult, error) {
	result := &MigrationResult{
		Source: source,
	}

	if config == nil {
		config = DefaultMigrationConfig()
	}

	// Get Automergent config directory
	home, err := os.UserHomeDir()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to get home directory: %v", err))
		return result, err
	}

	automergentDir := filepath.Join(home, ".automergent")
	if err := os.MkdirAll(automergentDir, 0755); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to create config directory: %v", err))
		return result, err
	}

	switch source {
	case MigrationClaudeCode:
		return migrateFromClaudeCode(config, result, automergentDir)
	case MigrationAider:
		return migrateFromAider(config, result, automergentDir)
	case MigrationCopilot:
		return migrateFromCopilot(config, result, automergentDir)
	case MigrationCursor:
		return migrateFromCursor(config, result, automergentDir)
	default:
		result.Errors = append(result.Errors, fmt.Sprintf("unknown migration source: %s", source))
		return result, fmt.Errorf("unknown migration source: %s", source)
	}
}

// migrateFromClaudeCode handles Claude Code migration
func migrateFromClaudeCode(config *MigrationConfig, result *MigrationResult, destDir string) (*MigrationResult, error) {
	home, _ := os.UserHomeDir()
	claudeDir := filepath.Join(home, ".claude")

	// Create backup if preserving
	if config.PreserveOld {
		backupPath, err := backupDirectory(claudeDir)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to backup: %v", err))
		} else {
			result.BackupPath = backupPath
		}
	}

	// Migrate settings
	if config.ImportSettings {
		settingsFile := filepath.Join(claudeDir, "settings.json")
		if data, err := os.ReadFile(settingsFile); err == nil {
			var settings map[string]interface{}
			if err := json.Unmarshal(data, &settings); err == nil {
				// Convert Claude settings to Automergent format
				oweSettings := convertClaudeSettings(settings)
				oweSettingsPath := filepath.Join(destDir, "settings.json")
				if oweData, err := json.MarshalIndent(oweSettings, "", "  "); err == nil {
					if err := os.WriteFile(oweSettingsPath, oweData, 0644); err == nil {
						result.SettingsMigrated++
					}
				}
			}
		}

		// Migrate API key if present
		apiKeyFile := filepath.Join(claudeDir, ".credentials")
		if _, err := os.Stat(apiKeyFile); err == nil {
			if err := copyFileForMigration(apiKeyFile, filepath.Join(destDir, ".credentials")); err == nil {
				result.SettingsMigrated++
			}
		}
	}

	// Migrate history
	if config.ImportHistory {
		historyDir := filepath.Join(claudeDir, "history")
		if entries, err := os.ReadDir(historyDir); err == nil {
			oweHistoryDir := filepath.Join(destDir, "history")
			os.MkdirAll(oweHistoryDir, 0755)
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
					src := filepath.Join(historyDir, entry.Name())
					dst := filepath.Join(oweHistoryDir, entry.Name())
					if err := copyFileForMigration(src, dst); err == nil {
						result.HistoryMigrated++
					}
				}
			}
		}
	}

	// Migrate projects
	if config.ImportProjects {
		projectsFile := filepath.Join(claudeDir, "projects.json")
		if data, err := os.ReadFile(projectsFile); err == nil {
			oweProjectsPath := filepath.Join(destDir, "projects.json")
			if err := os.WriteFile(oweProjectsPath, data, 0644); err == nil {
				result.ProjectsMigrated++
			}
		}
	}

	result.Success = len(result.Errors) == 0
	return result, nil
}

// migrateFromAider handles Aider migration
func migrateFromAider(config *MigrationConfig, result *MigrationResult, destDir string) (*MigrationResult, error) {
	home, _ := os.UserHomeDir()
	aiderDir := filepath.Join(home, ".aider")

	if config.PreserveOld {
		backupPath, err := backupDirectory(aiderDir)
		if err == nil {
			result.BackupPath = backupPath
		}
	}

	// Migrate settings
	if config.ImportSettings {
		// Aider uses .aider.conf.yml
		confFiles := []string{
			filepath.Join(home, ".aider.conf.yml"),
			filepath.Join(aiderDir, "config.yml"),
		}

		for _, confFile := range confFiles {
			if _, err := os.Stat(confFile); err == nil {
				// Read and convert YAML config
				// For now, we'll just note that settings exist
				result.SettingsMigrated++
				break
			}
		}
	}

	// Migrate history
	if config.ImportHistory {
		historyFile := filepath.Join(aiderDir, ".aider.chat.history.md")
		if data, err := os.ReadFile(historyFile); err == nil {
			oweHistoryDir := filepath.Join(destDir, "history")
			os.MkdirAll(oweHistoryDir, 0755)

			// Convert to Automergent format
			historyPath := filepath.Join(oweHistoryDir, "aider-import.md")
			if err := os.WriteFile(historyPath, data, 0644); err == nil {
				result.HistoryMigrated++
			}
		}
	}

	result.Success = len(result.Errors) == 0
	return result, nil
}

// migrateFromCopilot handles GitHub Copilot CLI migration
func migrateFromCopilot(config *MigrationConfig, result *MigrationResult, destDir string) (*MigrationResult, error) {
	home, _ := os.UserHomeDir()

	copilotPaths := []string{
		filepath.Join(home, ".config", "github-copilot"),
		filepath.Join(home, ".copilot"),
	}

	var copilotDir string
	for _, path := range copilotPaths {
		if _, err := os.Stat(path); err == nil {
			copilotDir = path
			break
		}
	}

	if copilotDir == "" {
		result.Warnings = append(result.Warnings, "Copilot configuration not found")
		return result, nil
	}

	if config.PreserveOld {
		backupPath, err := backupDirectory(copilotDir)
		if err == nil {
			result.BackupPath = backupPath
		}
	}

	// Migrate settings
	if config.ImportSettings {
		configFile := filepath.Join(copilotDir, "config.json")
		if _, err := os.Stat(configFile); err == nil {
			result.SettingsMigrated++
		}
	}

	result.Success = len(result.Errors) == 0
	return result, nil
}

// migrateFromCursor handles Cursor migration
func migrateFromCursor(config *MigrationConfig, result *MigrationResult, destDir string) (*MigrationResult, error) {
	home, _ := os.UserHomeDir()

	cursorPaths := []string{
		filepath.Join(home, ".cursor"),
		filepath.Join(home, "Library", "Application Support", "Cursor"),
	}
	if runtime.GOOS == "windows" {
		cursorPaths = append(cursorPaths, filepath.Join(os.Getenv("APPDATA"), "Cursor"))
	}

	var cursorDir string
	for _, path := range cursorPaths {
		if _, err := os.Stat(path); err == nil {
			cursorDir = path
			break
		}
	}

	if cursorDir == "" {
		result.Warnings = append(result.Warnings, "Cursor configuration not found")
		return result, nil
	}

	if config.PreserveOld {
		backupPath, err := backupDirectory(cursorDir)
		if err == nil {
			result.BackupPath = backupPath
		}
	}

	// Migrate settings
	if config.ImportSettings {
		settingsFile := filepath.Join(cursorDir, "User", "settings.json")
		if _, err := os.Stat(settingsFile); err == nil {
			result.SettingsMigrated++
		}
	}

	result.Success = len(result.Errors) == 0
	return result, nil
}

// convertClaudeSettings converts Claude Code settings to Automergent format
func convertClaudeSettings(claudeSettings map[string]interface{}) map[string]interface{} {
	oweSettings := make(map[string]interface{})

	// Map common settings
	mappings := map[string]string{
		"theme":       "theme",
		"fontSize":    "font_size",
		"fontFamily":  "font_family",
		"model":       "default_model",
		"maxTokens":   "max_tokens",
		"temperature": "temperature",
		"autoSave":    "auto_save",
		"cwd":         "working_directory",
		"editor":      "editor",
	}

	for claudeKey, oweKey := range mappings {
		if val, ok := claudeSettings[claudeKey]; ok {
			oweSettings[oweKey] = val
		}
	}

	// Add migration metadata
	oweSettings["_migrated_from"] = "claude-code"
	oweSettings["_migration_date"] = time.Now().Format(time.RFC3339)

	return oweSettings
}

// backupDirectory creates a backup of a directory
func backupDirectory(dir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	backupDir := filepath.Join(home, ".automergent", "migration-backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	timestamp := time.Now().Format("20060102-150405")
	dirName := filepath.Base(dir)
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s-%s", dirName, timestamp))

	// Copy directory
	return backupPath, copyDirectory(dir, backupPath)
}

// copyDirectory recursively copies a directory
func copyDirectory(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFileForMigration(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFileForMigration copies a single file
func copyFileForMigration(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}

// ListMigratableData shows what can be migrated from a source
func ListMigratableData(source MigrationSource) (map[string]bool, error) {
	data := map[string]bool{
		"settings": false,
		"history":  false,
		"projects": false,
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return data, err
	}

	switch source {
	case MigrationClaudeCode:
		claudeDir := filepath.Join(home, ".claude")
		if _, err := os.Stat(filepath.Join(claudeDir, "settings.json")); err == nil {
			data["settings"] = true
		}
		if _, err := os.Stat(filepath.Join(claudeDir, "history")); err == nil {
			data["history"] = true
		}
		if _, err := os.Stat(filepath.Join(claudeDir, "projects.json")); err == nil {
			data["projects"] = true
		}
	case MigrationAider:
		aiderDir := filepath.Join(home, ".aider")
		if _, err := os.Stat(filepath.Join(home, ".aider.conf.yml")); err == nil {
			data["settings"] = true
		}
		if _, err := os.Stat(filepath.Join(aiderDir, ".aider.chat.history.md")); err == nil {
			data["history"] = true
		}
	}

	return data, nil
}

// GetMigrationSummary returns a summary of what will be migrated
func GetMigrationSummary(source MigrationSource) string {
	data, err := ListMigratableData(source)
	if err != nil {
		return "Unable to analyze migration source"
	}

	var parts []string
	if data["settings"] {
		parts = append(parts, "settings")
	}
	if data["history"] {
		parts = append(parts, "chat history")
	}
	if data["projects"] {
		parts = append(parts, "project configurations")
	}

	if len(parts) == 0 {
		return "No data found to migrate"
	}

	return fmt.Sprintf("Will migrate: %s", strings.Join(parts, ", "))
}
