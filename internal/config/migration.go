package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// MigrationVersion represents a config version.
type MigrationVersion struct {
	Major int
	Minor int
	Patch int
}

// String returns the version string.
func (v MigrationVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1 if v < other, 0 if v == other, 1 if v > other.
func (v MigrationVersion) Compare(other MigrationVersion) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// ParseVersion parses a version string.
func ParseVersion(s string) (MigrationVersion, error) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")

	v := MigrationVersion{}
	if len(parts) >= 1 {
		v.Major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		v.Minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		v.Patch, _ = strconv.Atoi(parts[2])
	}

	return v, nil
}

// CurrentVersion is the current config schema version.
var CurrentVersion = MigrationVersion{Major: 2, Minor: 0, Patch: 0}

// Migration represents a config migration.
type Migration struct {
	FromVersion MigrationVersion
	ToVersion   MigrationVersion
	Description string
	Migrate     func(data map[string]any) (map[string]any, error)
	Rollback    func(data map[string]any) (map[string]any, error)
}

// Migrator handles config migrations.
type Migrator struct {
	migrations []Migration
	backupDir  string
}

// NewMigrator creates a new migrator with all registered migrations.
func NewMigrator() (*Migrator, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	backupDir := filepath.Join(home, ".automergent", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	m := &Migrator{
		backupDir: backupDir,
		migrations: []Migration{
			migration_1_0_to_1_1(),
			migration_1_1_to_1_2(),
			migration_1_2_to_2_0(),
		},
	}

	return m, nil
}

// migration_1_0_to_1_1 handles 1.0 -> 1.1 migration.
func migration_1_0_to_1_1() Migration {
	return Migration{
		FromVersion: MigrationVersion{1, 0, 0},
		ToVersion:   MigrationVersion{1, 1, 0},
		Description: "Add diagnostics config section",
		Migrate: func(data map[string]any) (map[string]any, error) {
			if _, ok := data["diagnostics"]; !ok {
				data["diagnostics"] = map[string]any{
					"enabled":          true,
					"showInRead":       true,
					"blockOnError":     true,
					"blockOnWarning":   false,
					"maxFileSizeBytes": 1 << 20,
					"cacheDurationSec": 30,
				}
			}
			return data, nil
		},
		Rollback: func(data map[string]any) (map[string]any, error) {
			delete(data, "diagnostics")
			return data, nil
		},
	}
}

// migration_1_1_to_1_2 handles 1.1 -> 1.2 migration.
func migration_1_1_to_1_2() Migration {
	return Migration{
		FromVersion: MigrationVersion{1, 1, 0},
		ToVersion:   MigrationVersion{1, 2, 0},
		Description: "Rename sandbox modes",
		Migrate: func(data map[string]any) (map[string]any, error) {
			if security, ok := data["security"].(map[string]any); ok {
				if sandbox, ok := security["sandbox"].(string); ok {
					// Rename old values
					switch sandbox {
					case "none":
						security["sandbox"] = "off"
					case "permissive":
						security["sandbox"] = "auto"
					case "full":
						security["sandbox"] = "strict"
					}
				}
			}
			return data, nil
		},
		Rollback: func(data map[string]any) (map[string]any, error) {
			if security, ok := data["security"].(map[string]any); ok {
				if sandbox, ok := security["sandbox"].(string); ok {
					switch sandbox {
					case "off":
						security["sandbox"] = "none"
					case "auto":
						security["sandbox"] = "permissive"
					case "strict":
						security["sandbox"] = "full"
					}
				}
			}
			return data, nil
		},
	}
}

// migration_1_2_to_2_0 handles 1.2 -> 2.0 migration (breaking changes).
func migration_1_2_to_2_0() Migration {
	return Migration{
		FromVersion: MigrationVersion{1, 2, 0},
		ToVersion:   MigrationVersion{2, 0, 0},
		Description: "Major restructuring: flatten provider config, add profiles support",
		Migrate: func(data map[string]any) (map[string]any, error) {
			// Move apiKey from top level to providers
			if apiKey, ok := data["apiKey"].(string); ok {
				provider := "google"
				if p, ok := data["provider"].(string); ok {
					provider = p
				}

				if data["providers"] == nil {
					data["providers"] = make(map[string]any)
				}
				providers := data["providers"].(map[string]any)
				if providers[provider] == nil {
					providers[provider] = make(map[string]any)
				}
				providerConfig := providers[provider].(map[string]any)
				providerConfig["apiKey"] = apiKey
				delete(data, "apiKey")
			}

			// Rename deprecated fields
			if oldField, ok := data["maxTokens"]; ok {
				data["maxContextTokens"] = oldField
				delete(data, "maxTokens")
			}

			// Add version marker
			data["_version"] = "2.0.0"

			return data, nil
		},
		Rollback: func(data map[string]any) (map[string]any, error) {
			// Move apiKey back to top level
			if providers, ok := data["providers"].(map[string]any); ok {
				provider := "google"
				if p, ok := data["provider"].(string); ok {
					provider = p
				}
				if pc, ok := providers[provider].(map[string]any); ok {
					if apiKey, ok := pc["apiKey"].(string); ok {
						data["apiKey"] = apiKey
					}
				}
			}

			// Rename fields back
			if newField, ok := data["maxContextTokens"]; ok {
				data["maxTokens"] = newField
				delete(data, "maxContextTokens")
			}

			delete(data, "_version")

			return data, nil
		},
	}
}

// Migrate migrates a config file to the latest version.
func (m *Migrator) Migrate(path string) error {
	// Read current config
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Detect version
	currentVersion := m.detectVersion(data)

	// Check if migration needed
	if currentVersion.Compare(CurrentVersion) >= 0 {
		return nil // Already at or above current version
	}

	// Create backup
	if err := m.createBackup(path, content); err != nil {
		return fmt.Errorf("create backup: %w", err)
	}

	// Apply migrations sequentially
	for _, migration := range m.migrations {
		if currentVersion.Compare(migration.FromVersion) >= 0 &&
			currentVersion.Compare(migration.ToVersion) < 0 {

			data, err = migration.Migrate(data)
			if err != nil {
				return fmt.Errorf("migration %s: %w", migration.Description, err)
			}
			currentVersion = migration.ToVersion
		}
	}

	// Write migrated config
	output, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, output, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// detectVersion detects the version of a config.
func (m *Migrator) detectVersion(data map[string]any) MigrationVersion {
	// Check explicit version marker
	if v, ok := data["_version"].(string); ok {
		ver, err := ParseVersion(v)
		if err == nil {
			return ver
		}
	}

	// Heuristic detection based on fields
	if _, ok := data["diagnostics"]; !ok {
		return MigrationVersion{1, 0, 0}
	}

	if security, ok := data["security"].(map[string]any); ok {
		if sandbox, ok := security["sandbox"].(string); ok {
			switch sandbox {
			case "none", "permissive", "full":
				return MigrationVersion{1, 1, 0}
			}
		}
	}

	if _, ok := data["apiKey"]; ok {
		return MigrationVersion{1, 2, 0}
	}

	return CurrentVersion
}

// createBackup creates a backup of the config file.
func (m *Migrator) createBackup(path string, content []byte) error {
	timestamp := time.Now().Format("20060102-150405")
	base := filepath.Base(path)
	backupPath := filepath.Join(m.backupDir, fmt.Sprintf("%s.%s.bak", base, timestamp))

	return os.WriteFile(backupPath, content, 0o644)
}

// ListBackups lists available config backups.
func (m *Migrator) ListBackups() ([]string, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, err
	}

	var backups []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".bak") {
			backups = append(backups, filepath.Join(m.backupDir, entry.Name()))
		}
	}

	return backups, nil
}

// Restore restores a config from backup.
func (m *Migrator) Restore(backupPath, targetPath string) error {
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}

	return os.WriteFile(targetPath, content, 0o644)
}

// ImportFormat represents a config format to import from.
type ImportFormat string

const (
	ImportClaudeCode ImportFormat = "claude-code"
	ImportCursor     ImportFormat = "cursor"
	ImportCopilot    ImportFormat = "copilot"
	ImportGeneric    ImportFormat = "generic"
)

// Import imports configuration from another tool.
func Import(format ImportFormat, sourcePath string) (*Config, error) {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}

	cfg := Default()

	switch format {
	case ImportClaudeCode:
		return importClaudeCode(content, cfg)
	case ImportCursor:
		return importCursor(content, cfg)
	case ImportCopilot:
		return importCopilot(content, cfg)
	case ImportGeneric:
		return importGeneric(content, cfg)
	}

	return nil, fmt.Errorf("unknown import format: %s", format)
}

// importClaudeCode imports Claude Code config.
func importClaudeCode(content []byte, cfg *Config) (*Config, error) {
	var data map[string]any

	// Try JSON first (Claude Code uses JSON)
	if err := json.Unmarshal(content, &data); err != nil {
		// Try YAML
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	// Map Claude Code fields to Automergent
	if model, ok := data["model"].(string); ok {
		cfg.Model = model
	}

	if theme, ok := data["theme"].(string); ok {
		cfg.Theme = theme
	}

	// Map Claude Code's allowed_tools to Automergent's tools config
	if allowedTools, ok := data["allowed_tools"].([]any); ok {
		for _, t := range allowedTools {
			if tool, ok := t.(string); ok {
				cfg.Tools[tool] = ToolConfig{Enabled: true}
			}
		}
	}

	// Map Claude Code's environment config
	if env, ok := data["environment"].(map[string]any); ok {
		if timeout, ok := env["command_timeout"].(string); ok {
			cfg.Tools["shell"] = ToolConfig{
				Enabled: true,
				Timeout: timeout,
			}
		}
	}

	// Map MCP servers
	if mcpServers, ok := data["mcpServers"].(map[string]any); ok {
		for name, serverData := range mcpServers {
			if server, ok := serverData.(map[string]any); ok {
				mcpServer := MCPServer{}
				if cmd, ok := server["command"].(string); ok {
					mcpServer.Command = []string{cmd}
				}
				if args, ok := server["args"].([]any); ok {
					for _, arg := range args {
						if a, ok := arg.(string); ok {
							mcpServer.Command = append(mcpServer.Command, a)
						}
					}
				}
				cfg.MCP.Servers[name] = mcpServer
			}
		}
	}

	return cfg, nil
}

// importCursor imports Cursor IDE config.
func importCursor(content []byte, cfg *Config) (*Config, error) {
	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Map Cursor AI settings
	if ai, ok := data["ai"].(map[string]any); ok {
		if model, ok := ai["model"].(string); ok {
			cfg.Model = model
		}
		if provider, ok := ai["provider"].(string); ok {
			cfg.Provider = strings.ToLower(provider)
		}
	}

	// Map editor settings
	if editor, ok := data["editor"].(map[string]any); ok {
		if theme, ok := editor["theme"].(string); ok {
			cfg.Theme = mapTheme(theme)
		}
	}

	return cfg, nil
}

// importCopilot imports GitHub Copilot config.
func importCopilot(content []byte, cfg *Config) (*Config, error) {
	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Map Copilot settings
	cfg.Provider = "google"

	if advanced, ok := data["advanced"].(map[string]any); ok {
		if debug, ok := advanced["debug"].(bool); ok && debug {
			cfg.Verbose = true
		}
	}

	return cfg, nil
}

// importGeneric imports a generic YAML/JSON config.
func importGeneric(content []byte, cfg *Config) (*Config, error) {
	var data map[string]any

	// Try JSON first
	if err := json.Unmarshal(content, &data); err != nil {
		// Try YAML
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	// Direct field mapping
	directFields := []string{
		"provider", "model", "mode", "theme", "keybindings", "layout",
		"autoSave", "maxSessions", "verbose", "quiet", "noColor",
	}

	for _, field := range directFields {
		if value, ok := data[field]; ok {
			_ = setConfigField(cfg, field, value)
		}
	}

	return cfg, nil
}

// mapTheme maps external themes to Automergent themes.
func mapTheme(external string) string {
	themeMap := map[string]string{
		"Dracula":      "dracula",
		"One Dark":     "onedark",
		"One Dark Pro": "onedark",
		"Nord":         "nord",
		"Gruvbox":      "gruvbox",
		"Gruvbox Dark": "gruvbox",
		"Monokai":      "monokai",
		"Monokai Pro":  "monokai",
		"Catppuccin":   "catppuccin",
		"Default":      "default",
	}

	if mapped, ok := themeMap[external]; ok {
		return mapped
	}

	// Try case-insensitive match
	lower := strings.ToLower(external)
	for ext, internal := range themeMap {
		if strings.ToLower(ext) == lower {
			return internal
		}
	}

	return "default"
}

// Export exports the current config.
func Export(cfg *Config, path string) error {
	ext := strings.ToLower(filepath.Ext(path))

	var data []byte
	var err error

	switch ext {
	case ".json":
		data, err = json.MarshalIndent(cfg, "", "  ")
	case ".yaml", ".yml":
		data, err = yaml.Marshal(cfg)
	default:
		data, err = yaml.Marshal(cfg)
	}

	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// DetectImportFormat attempts to detect the format of a config file.
func DetectImportFormat(path string) ImportFormat {
	content, err := os.ReadFile(path)
	if err != nil {
		return ImportGeneric
	}

	// Check for Claude Code markers
	if matched, _ := regexp.Match(`"allowed_tools"`, content); matched {
		return ImportClaudeCode
	}
	if matched, _ := regexp.Match(`"mcpServers"`, content); matched {
		return ImportClaudeCode
	}

	// Check for Cursor markers
	if matched, _ := regexp.Match(`"cursor\."`, content); matched {
		return ImportCursor
	}

	// Check for Copilot markers
	if matched, _ := regexp.Match(`"github\.copilot"`, content); matched {
		return ImportCopilot
	}

	return ImportGeneric
}
