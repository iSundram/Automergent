package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// Layer represents a configuration layer priority.
type Layer int

const (
	// LayerDefaults is the lowest priority, built-in defaults.
	LayerDefaults Layer = iota
	// LayerGlobal is user's global config (~/.automergent/config.yaml).
	LayerGlobal
	// LayerProject is project-specific config (.automergent/config.yaml).
	LayerProject
	// LayerProfile is named profile configuration.
	LayerProfile
	// LayerEnv is environment variables (AUTOMERGENT_*).
	LayerEnv
	// LayerSession is temporary session overrides.
	LayerSession
	// LayerCLI is CLI flags (highest priority).
	LayerCLI
)

// String returns the layer name.
func (l Layer) String() string {
	switch l {
	case LayerDefaults:
		return "defaults"
	case LayerGlobal:
		return "global"
	case LayerProject:
		return "project"
	case LayerProfile:
		return "profile"
	case LayerEnv:
		return "env"
	case LayerSession:
		return "session"
	case LayerCLI:
		return "cli"
	default:
		return "unknown"
	}
}

// LayerSource tracks where a config value came from.
type LayerSource struct {
	Layer Layer
	File  string // File path if loaded from file
	Key   string // Original key name
}

// Loader handles multi-layer configuration loading and merging.
type Loader struct {
	mu sync.RWMutex

	// Configuration state
	config      *Config
	layers      map[Layer]map[string]any
	sources     map[string]LayerSource
	globalPath  string
	projectPath string
	profileName string

	// Hot reload
	watcher   *fsnotify.Watcher
	reloadCh  chan struct{}
	onReload  []func(*Config)
	stopWatch chan struct{}

	// Schema validation
	schema *Schema
}

// LoaderOptions configures the loader behavior.
type LoaderOptions struct {
	// GlobalPath overrides the default global config path.
	GlobalPath string
	// ProjectDir is the project directory to search for .automergent/config.yaml.
	ProjectDir string
	// Profile selects a named profile to load.
	Profile string
	// EnableHotReload enables watching config files for changes.
	EnableHotReload bool
	// Schema enables schema validation.
	Schema *Schema
}

// NewLoader creates a new configuration loader.
func NewLoader(opts *LoaderOptions) (*Loader, error) {
	if opts == nil {
		opts = &LoaderOptions{}
	}

	l := &Loader{
		layers:    make(map[Layer]map[string]any),
		sources:   make(map[string]LayerSource),
		reloadCh:  make(chan struct{}, 1),
		stopWatch: make(chan struct{}),
		schema:    opts.Schema,
	}

	// Determine global config path
	if opts.GlobalPath != "" {
		l.globalPath = opts.GlobalPath
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		l.globalPath = filepath.Join(home, ".automergent", "config.yaml")
	}

	// Determine project config path
	if opts.ProjectDir != "" {
		l.projectPath = filepath.Join(opts.ProjectDir, ".automergent", "config.yaml")
	} else if wd, err := os.Getwd(); err == nil {
		l.projectPath = findProjectConfig(wd)
	}

	l.profileName = opts.Profile

	// Initialize hot reload if enabled
	if opts.EnableHotReload {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, fmt.Errorf("create watcher: %w", err)
		}
		l.watcher = watcher
		go l.watchLoop()
	}

	return l, nil
}

// findProjectConfig searches up the directory tree for .automergent/config.yaml.
func findProjectConfig(startDir string) string {
	dir := startDir
	for {
		configPath := filepath.Join(dir, ".automergent", "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// Load performs the full configuration loading sequence.
func (l *Loader) Load() (*Config, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Clear previous state
	l.layers = make(map[Layer]map[string]any)
	l.sources = make(map[string]LayerSource)

	// Load each layer in priority order
	if err := l.loadDefaults(); err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	if err := l.loadGlobal(); err != nil {
		return nil, fmt.Errorf("load global: %w", err)
	}

	if err := l.loadProject(); err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}

	if err := l.loadProfile(); err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}

	if err := l.loadEnv(); err != nil {
		return nil, fmt.Errorf("load env: %w", err)
	}

	// Merge all layers
	cfg, err := l.merge()
	if err != nil {
		return nil, fmt.Errorf("merge config: %w", err)
	}

	// Validate against schema
	if l.schema != nil {
		if errs := l.schema.Validate(cfg); len(errs) > 0 {
			return nil, fmt.Errorf("schema validation: %v", errs)
		}
	}

	l.config = cfg
	return cfg, nil
}

// loadDefaults loads built-in defaults.
func (l *Loader) loadDefaults() error {
	defaults := Default()
	data, err := configToMap(defaults)
	if err != nil {
		return err
	}
	flat := flattenMap(data)
	l.layers[LayerDefaults] = flat
	for k := range flat {
		l.sources[k] = LayerSource{Layer: LayerDefaults, Key: k}
	}
	return nil
}

// loadGlobal loads the global config file.
func (l *Loader) loadGlobal() error {
	if l.globalPath == "" {
		return nil
	}

	data, err := l.loadFile(l.globalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	flat := flattenMap(data)
	l.layers[LayerGlobal] = flat
	for k := range flat {
		l.sources[k] = LayerSource{Layer: LayerGlobal, File: l.globalPath, Key: k}
	}

	// Add to watcher
	if l.watcher != nil {
		_ = l.watcher.Add(l.globalPath)
	}

	return nil
}

// loadProject loads the project config file.
func (l *Loader) loadProject() error {
	if l.projectPath == "" {
		return nil
	}

	data, err := l.loadFile(l.projectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	flat := flattenMap(data)
	l.layers[LayerProject] = flat
	for k := range flat {
		l.sources[k] = LayerSource{Layer: LayerProject, File: l.projectPath, Key: k}
	}

	// Add to watcher
	if l.watcher != nil {
		_ = l.watcher.Add(l.projectPath)
	}

	return nil
}

// loadProfile loads a named profile.
func (l *Loader) loadProfile() error {
	if l.profileName == "" {
		return nil
	}

	// Check global profiles directory
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	profilePath := filepath.Join(home, ".automergent", "profiles", l.profileName+".yaml")
	data, err := l.loadFile(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("profile %q not found", l.profileName)
		}
		return err
	}

	flat := flattenMap(data)
	l.layers[LayerProfile] = flat
	for k := range flat {
		l.sources[k] = LayerSource{Layer: LayerProfile, File: profilePath, Key: k}
	}

	return nil
}

// loadEnv loads configuration from environment variables.
func (l *Loader) loadEnv() error {
	data := make(map[string]any)
	prefix := "AUTOMERGENT_"

	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, prefix) {
			continue
		}

		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimPrefix(parts[0], prefix))
		key = strings.ReplaceAll(key, "_", ".")
		value := parts[1]

		// Try to parse as JSON for complex values
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			data[key] = parsed
		} else {
			// Handle common types
			data[key] = parseEnvValue(value)
		}

		l.sources[key] = LayerSource{Layer: LayerEnv, Key: parts[0]}
	}

	l.layers[LayerEnv] = data
	return nil
}

// parseEnvValue converts string env value to appropriate type.
func parseEnvValue(value string) any {
	// Boolean
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}

	// Try int
	var intVal int
	if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
		return intVal
	}

	// Try float
	var floatVal float64
	if _, err := fmt.Sscanf(value, "%f", &floatVal); err == nil {
		return floatVal
	}

	return value
}

// SetSession sets session-level overrides (temporary).
func (l *Loader) SetSession(key string, value any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.layers[LayerSession] == nil {
		l.layers[LayerSession] = make(map[string]any)
	}
	l.layers[LayerSession][key] = value
	l.sources[key] = LayerSource{Layer: LayerSession, Key: key}
}

// ApplyCLIFlags applies CLI flags as the highest priority layer.
func (l *Loader) ApplyCLIFlags(flags *CLIFlags) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data := make(map[string]any)

	if flags.Provider != "" {
		data["provider"] = flags.Provider
	}
	if flags.Model != "" {
		data["model"] = flags.Model
	}
	if flags.Mode != "" {
		data["mode"] = flags.Mode
	}
	if flags.Theme != "" {
		data["theme"] = flags.Theme
	}
	if flags.Keybindings != "" {
		data["keybindings"] = flags.Keybindings
	}
	if flags.Layout != "" {
		data["layout"] = flags.Layout
	}
	if flags.SessionDir != "" {
		data["sessionDir"] = flags.SessionDir
	}
	if flags.NoTUI {
		data["noTui"] = true
	}
	if flags.NoColor {
		data["noColor"] = true
	}
	if flags.Quiet {
		data["quiet"] = true
	}
	if flags.Verbose {
		data["verbose"] = true
	}
	if flags.NoAnimation {
		data["noAnimation"] = true
	}
	if flags.NoSandbox {
		data["security.sandbox"] = "off"
	}
	if flags.Sandbox != "" {
		data["security.sandbox"] = flags.Sandbox
	}

	l.layers[LayerCLI] = data
	for k := range data {
		l.sources[k] = LayerSource{Layer: LayerCLI, Key: k}
	}
}

// loadFile loads a YAML, JSON, or TOML file.
func (l *Loader) loadFile(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(path))
	var data map[string]any

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(content, &data); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
	case ".toml":
		// TOML support using simple parser (avoiding external dependency)
		data, err = parseTOML(content)
		if err != nil {
			return nil, fmt.Errorf("parse toml: %w", err)
		}
	default:
		// Try YAML by default
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, fmt.Errorf("parse file: %w", err)
		}
	}

	return data, nil
}

// parseTOML provides basic TOML parsing for simple config files.
func parseTOML(content []byte) (map[string]any, error) {
	result := make(map[string]any)
	lines := strings.Split(string(content), "\n")
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[]")
			continue
		}

		// Key-value pair
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			value = value[1 : len(value)-1]
		}

		fullKey := key
		if currentSection != "" {
			fullKey = currentSection + "." + key
		}

		result[fullKey] = parseEnvValue(value)
	}

	return result, nil
}

// merge combines all layers into a final config.
func (l *Loader) merge() (*Config, error) {
	// Start with defaults
	cfg := Default()

	// Apply each layer in priority order
	for layer := LayerGlobal; layer <= LayerCLI; layer++ {
		data, ok := l.layers[layer]
		if !ok || len(data) == 0 {
			continue
		}

		if err := applyLayerToConfig(cfg, data); err != nil {
			return nil, fmt.Errorf("apply layer %s: %w", layer, err)
		}
	}

	return cfg, nil
}

// applyLayerToConfig applies a map of settings to a config struct.
func applyLayerToConfig(cfg *Config, data map[string]any) error {
	for key, value := range data {
		if err := SetConfigField(cfg, key, value); err != nil {
			// Log but don't fail on unknown keys
			continue
		}
	}
	return nil
}

// flattenMap converts nested map values into dot-notation keys.
func flattenMap(data map[string]any) map[string]any {
	out := make(map[string]any)
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		switch v := value.(type) {
		case map[string]any:
			for k, child := range v {
				next := k
				if prefix != "" {
					next = prefix + "." + k
				}
				walk(next, child)
			}
		case map[any]any:
			for k, child := range v {
				key := fmt.Sprintf("%v", k)
				next := key
				if prefix != "" {
					next = prefix + "." + key
				}
				walk(next, child)
			}
		default:
			if prefix != "" {
				out[prefix] = v
			}
		}
	}
	for k, v := range data {
		walk(k, v)
	}
	return out
}

// SetConfigField sets a config field by path (e.g., "security.sandbox").
func SetConfigField(cfg *Config, path string, value any) error {
	parts := strings.Split(path, ".")

	// Handle top-level fields
	if len(parts) == 1 {
		return setTopLevelField(cfg, path, value)
	}

	// Handle nested fields
	switch parts[0] {
	case "security":
		return setSecurityField(&cfg.Security, parts[1], value)
	case "log":
		return setLogField(&cfg.Log, parts[1], value)
	case "lsp":
		return setLSPField(&cfg.LSP, parts[1], value)
	case "notifications":
		return setNotificationField(&cfg.Notifications, parts[1], value)
	case "diagnostics":
		return setDiagnosticsField(&cfg.Diagnostics, parts[1], value)
	case "coordinator":
		return setCoordinatorField(&cfg.Coordinator, parts[1], value)
	}

	return fmt.Errorf("unknown field: %s", path)
}

// setTopLevelField sets a top-level config field.
func setTopLevelField(cfg *Config, key string, value any) error {
	switch key {
	case "provider":
		cfg.Provider = toString(value)
	case "model":
		cfg.Model = toString(value)
	case "mode":
		cfg.Mode = toString(value)
	case "theme":
		cfg.Theme = toString(value)
	case "keybindings":
		cfg.Keybindings = toString(value)
	case "layout":
		cfg.Layout = toString(value)
	case "autoSave":
		cfg.AutoSave = toBool(value)
	case "checkpointInterval":
		cfg.CheckpointInterval = toInt(value)
	case "sessionDir":
		cfg.SessionDir = toString(value)
	case "maxSessions":
		cfg.MaxSessions = toInt(value)
	case "maxSessionAge":
		cfg.MaxSessionAge = toString(value)
	case "maxContextTokens":
		cfg.MaxContextTokens = toInt(value)
	case "warnAtContextFraction":
		cfg.WarnAtContextFraction = toFloat(value)
	case "autoCompressAt":
		cfg.AutoCompressAt = toFloat(value)
	case "compressionKeepRecent":
		cfg.CompressionKeepRecent = toInt(value)
	case "maxAutoReadFileSize":
		cfg.MaxAutoReadFileSize = toInt(value)
	case "maxTreeFiles":
		cfg.MaxTreeFiles = toInt(value)
	case "maxTreeDepth":
		cfg.MaxTreeDepth = toInt(value)
	case "noAnimation":
		cfg.NoAnimation = toBool(value)
	case "noColor":
		cfg.NoColor = toBool(value)
	case "noTui":
		cfg.NoTUI = toBool(value)
	case "quiet":
		cfg.Quiet = toBool(value)
	case "verbose":
		cfg.Verbose = toBool(value)
	case "reasoningPreAnalysis":
		cfg.ReasoningPreAnalysis = toBool(value)
	case "promptSystemEnabled":
		cfg.PromptSystemEnabled = toBool(value)
	case "zeroDataRetention":
		cfg.ZeroDataRetention = toBool(value)
	case "telemetry":
		cfg.Telemetry = toBool(value)
	case "noUpdateCheck":
		cfg.NoUpdateCheck = toBool(value)
	case "debug":
		return setDebugField(&cfg.Debug, value)
	default:
		return fmt.Errorf("unknown field: %s", key)
	}
	return nil
}

// setSecurityField sets a security config field.
func setSecurityField(sec *SecurityConfig, key string, value any) error {
	switch key {
	case "sandbox":
		sec.Sandbox = toString(value)
	case "requireGitForAutoModes":
		sec.RequireGitForAutoModes = toBool(value)
	case "rootRiskAcknowledged":
		sec.RootRiskAcknowledged = toBool(value)
	default:
		return fmt.Errorf("unknown security field: %s", key)
	}
	return nil
}

// setLogField sets a log config field.
func setLogField(log *LogConfig, key string, value any) error {
	switch key {
	case "level":
		log.Level = toString(value)
	case "file":
		log.File = toString(value)
	case "maxSize":
		log.MaxSize = toString(value)
	case "maxBackups":
		log.MaxBackups = toInt(value)
	default:
		return fmt.Errorf("unknown log field: %s", key)
	}
	return nil
}

// setDebugField sets a debug config field.
func setDebugField(debug *DebugConfig, value any) error {
	m, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("debug config must be a map")
	}
	for k, v := range m {
		switch k {
		case "enabled":
			debug.Enabled = toBool(v)
		case "directory":
			debug.Directory = toString(v)
		case "saveRequests":
			debug.SaveRequests = toBool(v)
		case "saveResponses":
			debug.SaveResponses = toBool(v)
		case "maxFileSize":
			debug.MaxFileSize = toInt(v)
		default:
			return fmt.Errorf("unknown debug field: %s", k)
		}
	}
	return nil
}

// setLSPField sets an LSP config field.
func setLSPField(lsp *LSPConfig, key string, value any) error {
	switch key {
	case "enabled":
		lsp.Enabled = toBool(value)
	case "startupTimeout":
		lsp.StartupTimeout = toString(value)
	case "requestTimeout":
		lsp.RequestTimeout = toString(value)
	default:
		return fmt.Errorf("unknown lsp field: %s", key)
	}
	return nil
}

// setNotificationField sets a notification config field.
func setNotificationField(n *NotificationConfig, key string, value any) error {
	switch key {
	case "desktop":
		n.Desktop = toBool(value)
	case "bell":
		n.Bell = toBool(value)
	case "contextWarning":
		n.ContextWarning = toBool(value)
	default:
		return fmt.Errorf("unknown notification field: %s", key)
	}
	return nil
}

// setDiagnosticsField sets a diagnostics config field.
func setDiagnosticsField(d *DiagnosticsConfig, key string, value any) error {
	switch key {
	case "enabled":
		d.Enabled = toBool(value)
	case "showInRead":
		d.ShowInRead = toBool(value)
	case "blockOnError":
		d.BlockOnError = toBool(value)
	case "blockOnWarning":
		d.BlockOnWarning = toBool(value)
	case "maxFileSizeBytes":
		d.MaxFileSizeBytes = int64(toInt(value))
	case "cacheDurationSec":
		d.CacheDurationSec = toInt(value)
	default:
		return fmt.Errorf("unknown diagnostics field: %s", key)
	}
	return nil
}

// setCoordinatorField sets a coordinator config field.
func setCoordinatorField(c *CoordinatorConfig, key string, value any) error {
	switch key {
	case "enabled":
		c.Enabled = toBool(value)
	case "defaultTimeout":
		c.DefaultTimeout = toString(value)
	case "maxRetries":
		c.MaxRetries = toInt(value)
	case "qualityThreshold":
		c.QualityThreshold = toFloat(value)
	case "consensusThreshold":
		c.ConsensusThreshold = toInt(value)
	case "resourceLimits.maxTokensPerTask":
		c.ResourceLimits.MaxTokensPerTask = toInt(value)
	case "resourceLimits.maxConcurrentTasks":
		c.ResourceLimits.MaxConcurrentTasks = toInt(value)
	case "resourceLimits.maxMemoryMB":
		c.ResourceLimits.MaxMemoryMB = toInt(value)
	case "resourceLimits.rateLimitPerMinute":
		c.ResourceLimits.RateLimitPerMinute = toInt(value)
	default:
		return fmt.Errorf("unknown coordinator field: %s", key)
	}
	return nil
}

// Type conversion helpers
func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.ToLower(val) == "true" || val == "1"
	case int:
		return val != 0
	default:
		return false
	}
}

func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

// configToMap converts a Config to a map for layer storage.
func configToMap(cfg *Config) (map[string]any, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetSource returns the source of a config value.
func (l *Loader) GetSource(key string) (LayerSource, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	src, ok := l.sources[key]
	return src, ok
}

// Sources returns a snapshot of key->source mappings.
func (l *Loader) Sources() map[string]LayerSource {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[string]LayerSource, len(l.sources))
	for k, v := range l.sources {
		out[k] = v
	}
	return out
}

// Layers returns a snapshot of layer values.
func (l *Loader) Layers() map[Layer]map[string]any {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[Layer]map[string]any, len(l.layers))
	for layer, values := range l.layers {
		cloned := make(map[string]any, len(values))
		for k, v := range values {
			cloned[k] = v
		}
		out[layer] = cloned
	}
	return out
}

// watchLoop handles hot reload of config files.
func (l *Loader) watchLoop() {
	debounce := time.NewTimer(0)
	<-debounce.C // Drain initial timer

	for {
		select {
		case <-l.stopWatch:
			return
		case event, ok := <-l.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// Debounce rapid changes
				debounce.Reset(100 * time.Millisecond)
			}
		case <-debounce.C:
			// Reload config
			cfg, err := l.Load()
			if err != nil {
				continue
			}

			// Get callbacks under lock to avoid race condition
			l.mu.RLock()
			callbacks := make([]func(*Config), len(l.onReload))
			copy(callbacks, l.onReload)
			l.mu.RUnlock()

			// Call callbacks outside lock to avoid deadlock
			for _, fn := range callbacks {
				fn(cfg)
			}
		case err, ok := <-l.watcher.Errors:
			if !ok {
				return
			}
			_ = err // Log error in production
		}
	}
}

// OnReload registers a callback for config changes.
func (l *Loader) OnReload(fn func(*Config)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onReload = append(l.onReload, fn)
}

// ReloadCh returns a channel that signals config reloads.
func (l *Loader) ReloadCh() <-chan struct{} {
	return l.reloadCh
}

// Close stops the hot reload watcher.
func (l *Loader) Close() error {
	close(l.stopWatch)
	if l.watcher != nil {
		return l.watcher.Close()
	}
	return nil
}

// Config returns the current loaded config.
func (l *Loader) Config() *Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.config
}

// Errors for config loading
var (
	ErrNoConfig        = errors.New("no configuration found")
	ErrInvalidConfig   = errors.New("invalid configuration")
	ErrProfileNotFound = errors.New("profile not found")
)
