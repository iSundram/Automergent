package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

// Orchestrator manages multiple MCP servers with hot-reload support.
type Orchestrator struct {
	config    OrchestratorConfig
	servers   map[string]*ManagedServer
	mu        sync.RWMutex
	toolIndex map[string][]ToolInfo // tool name -> servers providing it
	indexMu   sync.RWMutex

	// Load balancing
	roundRobinIdx map[string]int
	rrMu          sync.Mutex

	// Hot-reload
	watcher        *fsnotify.Watcher
	configPath     string
	watcherDone    chan struct{}
	onConfigChange ConfigChangeHandler
	onServerEvent  EventHandler

	// Cache
	cache   *responseCache
	cacheMu sync.RWMutex

	// Lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	started   bool
	startedMu sync.Mutex
}

// responseCache caches tool responses.
type responseCache struct {
	entries map[string]*cacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
}

type cacheEntry struct {
	result    *ToolResult
	expiresAt time.Time
}

func newResponseCache(ttl time.Duration) *responseCache {
	return &responseCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *responseCache) get(key string) (*ToolResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.result, true
}

func (c *responseCache) set(key string, result *ToolResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *responseCache) invalidate(pattern string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.entries {
		if pattern == "*" || key == pattern {
			delete(c.entries, key)
		}
	}
}

// NewOrchestrator creates a new multi-server orchestrator.
func NewOrchestrator(config OrchestratorConfig) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())

	cacheTTL := config.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}

	return &Orchestrator{
		config:        config,
		servers:       make(map[string]*ManagedServer),
		toolIndex:     make(map[string][]ToolInfo),
		roundRobinIdx: make(map[string]int),
		cache:         newResponseCache(cacheTTL),
		ctx:           ctx,
		cancel:        cancel,
		watcherDone:   make(chan struct{}),
	}
}

// SetEventHandlers sets the event handlers for the orchestrator.
func (o *Orchestrator) SetEventHandlers(onServerEvent EventHandler, onConfigChange ConfigChangeHandler) {
	o.onServerEvent = onServerEvent
	o.onConfigChange = onConfigChange
}

// Start initializes and connects to all configured servers.
func (o *Orchestrator) Start(ctx context.Context) error {
	o.startedMu.Lock()
	if o.started {
		o.startedMu.Unlock()
		return fmt.Errorf("orchestrator already started")
	}
	o.started = true
	o.startedMu.Unlock()

	// Connect to enabled servers concurrently
	var wg sync.WaitGroup
	errCh := make(chan error, len(o.config.Servers))

	for i := range o.config.Servers {
		cfg := &o.config.Servers[i]
		if !cfg.Enabled {
			continue
		}

		wg.Add(1)
		go func(cfg *ServerConfig) {
			defer wg.Done()
			if err := o.addServer(ctx, cfg); err != nil {
				errCh <- fmt.Errorf("server %s: %w", cfg.Name, err)
			}
		}(cfg)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	// Build initial tool index
	o.rebuildToolIndex()

	if len(errs) > 0 && len(errs) == len(o.config.Servers) {
		return fmt.Errorf("failed to connect to any server: %v", errs)
	}

	return nil
}

// addServer adds and connects to a new server.
func (o *Orchestrator) addServer(ctx context.Context, cfg *ServerConfig) error {
	server := NewManagedServer(cfg, o.handleServerEvent)

	connectCtx := ctx
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		connectCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	if err := server.Connect(connectCtx); err != nil {
		// Retry if configured
		for i := 0; i < cfg.RetryCount; i++ {
			time.Sleep(time.Second * time.Duration(i+1))
			if err = server.Connect(connectCtx); err == nil {
				break
			}
		}
		if err != nil {
			return err
		}
	}

	o.mu.Lock()
	o.servers[cfg.Name] = server
	o.mu.Unlock()

	return nil
}

// handleServerEvent processes events from managed servers.
func (o *Orchestrator) handleServerEvent(event ServerEvent) {
	// Update tool index on tool changes
	if event.Type == EventToolsUpdated {
		o.rebuildToolIndex()
	}

	// Forward to external handler
	if o.onServerEvent != nil {
		o.onServerEvent(event)
	}
}

// rebuildToolIndex rebuilds the tool-to-server mapping.
func (o *Orchestrator) rebuildToolIndex() {
	o.mu.RLock()
	defer o.mu.RUnlock()

	newIndex := make(map[string][]ToolInfo)

	for _, server := range o.servers {
		if !server.IsHealthy() {
			continue
		}

		for _, tool := range server.Tools() {
			// Index by both simple name and qualified name
			newIndex[tool.Name] = append(newIndex[tool.Name], tool)
			if tool.QualifiedName != tool.Name {
				newIndex[tool.QualifiedName] = append(newIndex[tool.QualifiedName], tool)
			}
		}
	}

	// Sort by priority (highest first)
	for name := range newIndex {
		sort.Slice(newIndex[name], func(i, j int) bool {
			return newIndex[name][i].Priority > newIndex[name][j].Priority
		})
	}

	o.indexMu.Lock()
	o.toolIndex = newIndex
	o.indexMu.Unlock()
}

// Stop gracefully shuts down all servers.
func (o *Orchestrator) Stop() error {
	o.cancel()

	// Stop config watcher
	if o.watcher != nil {
		o.watcher.Close()
		<-o.watcherDone
	}

	// Disconnect all servers
	o.mu.Lock()
	defer o.mu.Unlock()

	var lastErr error
	for name, server := range o.servers {
		if err := server.Close(); err != nil {
			lastErr = fmt.Errorf("close %s: %w", name, err)
		}
	}

	o.servers = make(map[string]*ManagedServer)
	return lastErr
}

// CallTool executes a tool, automatically routing to the appropriate server.
func (o *Orchestrator) CallTool(ctx context.Context, call ToolCall) (*ToolResult, error) {
	// Check cache first
	if o.config.CacheEnabled {
		cacheKey := o.cacheKey(call)
		if result, ok := o.cache.get(cacheKey); ok {
			return result, nil
		}
	}

	// Find server for the tool
	server, err := o.selectServer(call.Name, call.Server)
	if err != nil {
		return nil, err
	}

	// Execute the tool
	result, err := server.CallTool(ctx, call.Name, call.Args)
	if err != nil {
		// Try failover if enabled
		if o.config.FailoverEnabled && call.Server == "" {
			result, err = o.tryFailover(ctx, call, server.Config().Name)
		}
		if err != nil {
			return nil, err
		}
	}

	// Cache successful results
	if o.config.CacheEnabled && !result.IsError {
		o.cache.set(o.cacheKey(call), result)
	}

	return result, nil
}

// selectServer selects a server for the given tool.
func (o *Orchestrator) selectServer(toolName, preferredServer string) (*ManagedServer, error) {
	// If specific server requested
	if preferredServer != "" {
		o.mu.RLock()
		server, ok := o.servers[preferredServer]
		o.mu.RUnlock()

		if !ok {
			return nil, fmt.Errorf("server %s not found", preferredServer)
		}
		if !server.IsHealthy() {
			return nil, fmt.Errorf("server %s is unhealthy", preferredServer)
		}
		return server, nil
	}

	// Find servers providing the tool
	o.indexMu.RLock()
	tools, ok := o.toolIndex[toolName]
	o.indexMu.RUnlock()

	if !ok || len(tools) == 0 {
		return nil, fmt.Errorf("tool %s not found", toolName)
	}

	// Filter to healthy servers
	var healthyTools []ToolInfo
	o.mu.RLock()
	for _, tool := range tools {
		if server, ok := o.servers[tool.Server]; ok && server.IsHealthy() {
			healthyTools = append(healthyTools, tool)
		}
	}
	o.mu.RUnlock()

	if len(healthyTools) == 0 {
		return nil, fmt.Errorf("no healthy server provides tool %s", toolName)
	}

	// Select using load balancing strategy
	var selectedTool ToolInfo

	switch o.config.LoadBalancing {
	case LoadBalanceRoundRobin:
		selectedTool = o.selectRoundRobin(toolName, healthyTools)
	case LoadBalanceLeastLatency:
		selectedTool = o.selectLeastLatency(healthyTools)
	case LoadBalanceRandom:
		selectedTool = healthyTools[rand.Intn(len(healthyTools))]
	case LoadBalancePriority:
		fallthrough
	default:
		selectedTool = healthyTools[0] // Already sorted by priority
	}

	o.mu.RLock()
	server := o.servers[selectedTool.Server]
	o.mu.RUnlock()

	return server, nil
}

func (o *Orchestrator) selectRoundRobin(toolName string, tools []ToolInfo) ToolInfo {
	o.rrMu.Lock()
	defer o.rrMu.Unlock()

	idx := o.roundRobinIdx[toolName]
	o.roundRobinIdx[toolName] = (idx + 1) % len(tools)
	return tools[idx%len(tools)]
}

func (o *Orchestrator) selectLeastLatency(tools []ToolInfo) ToolInfo {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var best ToolInfo
	var bestLatency time.Duration = -1

	for _, tool := range tools {
		if server, ok := o.servers[tool.Server]; ok {
			info := server.Info()
			if bestLatency < 0 || info.Latency < bestLatency {
				best = tool
				bestLatency = info.Latency
			}
		}
	}

	return best
}

// tryFailover attempts to execute on another server.
func (o *Orchestrator) tryFailover(ctx context.Context, call ToolCall, failedServer string) (*ToolResult, error) {
	o.indexMu.RLock()
	tools := o.toolIndex[call.Name]
	o.indexMu.RUnlock()

	o.mu.RLock()
	defer o.mu.RUnlock()

	for _, tool := range tools {
		if tool.Server == failedServer {
			continue
		}

		server, ok := o.servers[tool.Server]
		if !ok || !server.IsHealthy() {
			continue
		}

		result, err := server.CallTool(ctx, call.Name, call.Args)
		if err == nil {
			return result, nil
		}
	}

	return nil, fmt.Errorf("all servers failed for tool %s", call.Name)
}

func (o *Orchestrator) cacheKey(call ToolCall) string {
	argsJSON, _ := json.Marshal(call.Args)
	return fmt.Sprintf("%s:%s:%s", call.Server, call.Name, string(argsJSON))
}

// ListTools returns all available tools from all servers.
func (o *Orchestrator) ListTools() []ToolInfo {
	o.indexMu.RLock()
	defer o.indexMu.RUnlock()

	// Deduplicate by qualified name
	seen := make(map[string]bool)
	var tools []ToolInfo

	for _, toolList := range o.toolIndex {
		for _, tool := range toolList {
			if seen[tool.QualifiedName] {
				continue
			}
			seen[tool.QualifiedName] = true
			tools = append(tools, tool)
		}
	}

	// Sort by name
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].QualifiedName < tools[j].QualifiedName
	})

	return tools
}

// GetTool returns information about a specific tool.
func (o *Orchestrator) GetTool(name string) (ToolInfo, bool) {
	o.indexMu.RLock()
	defer o.indexMu.RUnlock()

	tools, ok := o.toolIndex[name]
	if !ok || len(tools) == 0 {
		return ToolInfo{}, false
	}
	return tools[0], true
}

// ListServers returns information about all servers.
func (o *Orchestrator) ListServers() []ServerInfo {
	o.mu.RLock()
	defer o.mu.RUnlock()

	servers := make([]ServerInfo, 0, len(o.servers))
	for _, server := range o.servers {
		servers = append(servers, server.Info())
	}

	// Sort by name
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Config.Name < servers[j].Config.Name
	})

	return servers
}

// GetServer returns information about a specific server.
func (o *Orchestrator) GetServer(name string) (ServerInfo, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	server, ok := o.servers[name]
	if !ok {
		return ServerInfo{}, false
	}
	return server.Info(), true
}

// HealthySummary returns a summary of server health.
func (o *Orchestrator) HealthySummary() map[string]ServerStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()

	summary := make(map[string]ServerStatus)
	for name, server := range o.servers {
		info := server.Info()
		summary[name] = info.Status
	}
	return summary
}

// ========== Hot Reload ==========

// WatchConfig starts watching a configuration file for changes.
func (o *Orchestrator) WatchConfig(configPath string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	o.watcher = watcher
	o.configPath = configPath
	o.watcherDone = make(chan struct{})

	// Watch the directory containing the config file
	dir := filepath.Dir(configPath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return fmt.Errorf("watch directory: %w", err)
	}

	go o.watchLoop()

	return nil
}

func (o *Orchestrator) watchLoop() {
	defer close(o.watcherDone)

	for {
		select {
		case <-o.ctx.Done():
			return
		case event, ok := <-o.watcher.Events:
			if !ok {
				return
			}

			// Check if it's our config file
			if filepath.Clean(event.Name) != filepath.Clean(o.configPath) {
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// Debounce rapid changes
				time.Sleep(100 * time.Millisecond)
				o.reloadConfig()
			}

		case err, ok := <-o.watcher.Errors:
			if !ok {
				return
			}
			if o.onServerEvent != nil {
				o.onServerEvent(ServerEvent{
					Type:      EventError,
					Error:     fmt.Sprintf("config watcher: %v", err),
					Timestamp: time.Now(),
				})
			}
		}
	}
}

// reloadConfig reloads configuration from the watched file.
func (o *Orchestrator) reloadConfig() {
	data, err := os.ReadFile(o.configPath)
	if err != nil {
		if o.onServerEvent != nil {
			o.onServerEvent(ServerEvent{
				Type:      EventError,
				Error:     fmt.Sprintf("read config: %v", err),
				Timestamp: time.Now(),
			})
		}
		return
	}

	var newConfig OrchestratorConfig
	if err := yaml.Unmarshal(data, &newConfig); err != nil {
		if o.onServerEvent != nil {
			o.onServerEvent(ServerEvent{
				Type:      EventError,
				Error:     fmt.Sprintf("parse config: %v", err),
				Timestamp: time.Now(),
			})
		}
		return
	}

	o.ApplyConfig(newConfig)
}

// ApplyConfig applies a new configuration, handling adds/updates/removes.
func (o *Orchestrator) ApplyConfig(newConfig OrchestratorConfig) {
	o.mu.Lock()
	oldServers := make(map[string]*ServerConfig)
	for _, cfg := range o.config.Servers {
		oldServers[cfg.Name] = &cfg
	}
	o.mu.Unlock()

	newServers := make(map[string]*ServerConfig)
	for i := range newConfig.Servers {
		newServers[newConfig.Servers[i].Name] = &newConfig.Servers[i]
	}

	// Find changes
	var toAdd, toUpdate, toRemove []string

	for name := range newServers {
		if _, exists := oldServers[name]; !exists {
			toAdd = append(toAdd, name)
		} else if !serverConfigEqual(oldServers[name], newServers[name]) {
			toUpdate = append(toUpdate, name)
		}
	}

	for name := range oldServers {
		if _, exists := newServers[name]; !exists {
			toRemove = append(toRemove, name)
		}
	}

	// Apply changes
	ctx := context.Background()

	// Remove old servers
	for _, name := range toRemove {
		o.RemoveServer(name)
		if o.onConfigChange != nil {
			o.onConfigChange(ConfigChangeEvent{
				Type:      ConfigRemoved,
				Server:    name,
				OldConfig: oldServers[name],
				Timestamp: time.Now(),
			})
		}
	}

	// Update existing servers
	for _, name := range toUpdate {
		cfg := newServers[name]
		o.UpdateServer(ctx, cfg)
		if o.onConfigChange != nil {
			o.onConfigChange(ConfigChangeEvent{
				Type:      ConfigUpdated,
				Server:    name,
				OldConfig: oldServers[name],
				NewConfig: cfg,
				Timestamp: time.Now(),
			})
		}
	}

	// Add new servers
	for _, name := range toAdd {
		cfg := newServers[name]
		if cfg.Enabled {
			o.addServer(ctx, cfg)
		}
		if o.onConfigChange != nil {
			o.onConfigChange(ConfigChangeEvent{
				Type:      ConfigAdded,
				Server:    name,
				NewConfig: cfg,
				Timestamp: time.Now(),
			})
		}
	}

	// Update config
	o.mu.Lock()
	o.config = newConfig
	o.mu.Unlock()

	// Rebuild tool index
	o.rebuildToolIndex()
}

// AddServer adds a new server at runtime.
func (o *Orchestrator) AddServer(ctx context.Context, cfg *ServerConfig) error {
	o.mu.RLock()
	_, exists := o.servers[cfg.Name]
	o.mu.RUnlock()

	if exists {
		return fmt.Errorf("server %s already exists", cfg.Name)
	}

	if err := o.addServer(ctx, cfg); err != nil {
		return err
	}

	// Add to config
	o.mu.Lock()
	o.config.Servers = append(o.config.Servers, *cfg)
	o.mu.Unlock()

	o.rebuildToolIndex()
	return nil
}

// RemoveServer removes a server at runtime.
func (o *Orchestrator) RemoveServer(name string) error {
	o.mu.Lock()
	server, ok := o.servers[name]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("server %s not found", name)
	}
	delete(o.servers, name)

	// Remove from config
	newServers := make([]ServerConfig, 0, len(o.config.Servers))
	for _, cfg := range o.config.Servers {
		if cfg.Name != name {
			newServers = append(newServers, cfg)
		}
	}
	o.config.Servers = newServers
	o.mu.Unlock()

	server.Close()
	o.rebuildToolIndex()
	return nil
}

// UpdateServer updates a server configuration (reconnects if needed).
func (o *Orchestrator) UpdateServer(ctx context.Context, cfg *ServerConfig) error {
	o.mu.RLock()
	server, ok := o.servers[cfg.Name]
	o.mu.RUnlock()

	if !ok {
		// Server doesn't exist, add it
		if cfg.Enabled {
			return o.AddServer(ctx, cfg)
		}
		return nil
	}

	// Check if we need to reconnect
	oldConfig := server.Config()
	needsReconnect := oldConfig.URL != cfg.URL ||
		oldConfig.Command != cfg.Command ||
		oldConfig.Transport != cfg.Transport ||
		oldConfig.AuthToken != cfg.AuthToken

	if !cfg.Enabled {
		return o.RemoveServer(cfg.Name)
	}

	if needsReconnect {
		// Close old, create new
		server.Close()

		newServer := NewManagedServer(cfg, o.handleServerEvent)
		if err := newServer.Connect(ctx); err != nil {
			return err
		}

		o.mu.Lock()
		o.servers[cfg.Name] = newServer
		o.mu.Unlock()
	} else {
		// Just update config
		server.UpdateConfig(cfg)
	}

	o.rebuildToolIndex()
	return nil
}

// RefreshServer refreshes tool discovery for a server.
func (o *Orchestrator) RefreshServer(ctx context.Context, name string) error {
	o.mu.RLock()
	server, ok := o.servers[name]
	o.mu.RUnlock()

	if !ok {
		return fmt.Errorf("server %s not found", name)
	}

	if err := server.RefreshTools(ctx); err != nil {
		return err
	}

	o.rebuildToolIndex()
	return nil
}

// InvalidateCache invalidates cached responses.
func (o *Orchestrator) InvalidateCache(pattern string) {
	if o.config.CacheEnabled {
		o.cache.invalidate(pattern)
	}
}

// serverConfigEqual compares two server configs for equality.
func serverConfigEqual(a, b *ServerConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.URL == b.URL &&
		a.Command == b.Command &&
		a.Transport == b.Transport &&
		a.AuthToken == b.AuthToken &&
		a.Priority == b.Priority &&
		a.Namespace == b.Namespace &&
		a.Enabled == b.Enabled &&
		a.Timeout == b.Timeout
}
