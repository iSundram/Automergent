package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ManagedServer wraps a transport with management capabilities.
type ManagedServer struct {
	config    *ServerConfig
	transport Transport
	info      *ServerInfo
	mu        sync.RWMutex

	// Metrics
	requestCount atomic.Int64
	errorCount   atomic.Int64
	totalLatency atomic.Int64

	// Health monitoring
	healthCancel context.CancelFunc
	healthDone   chan struct{}

	// Event handlers
	onEvent EventHandler

	// Event queue to avoid spawning unbounded goroutines for events
	eventCh chan ServerEvent
	eventWg sync.WaitGroup
}

// NewManagedServer creates a new managed server instance.
func NewManagedServer(config *ServerConfig, onEvent EventHandler) *ManagedServer {
	ms := &ManagedServer{
		config:     config,
		transport:  CreateTransport(config),
		onEvent:    onEvent,
		healthDone: make(chan struct{}),
		eventCh:    make(chan ServerEvent, 16),
		info: &ServerInfo{
			Config:       config,
			Status:       StatusDisconnected,
			Tools:        []ToolInfo{},
			Capabilities: make(map[string]bool),
		},
	}

	// Start a background dispatcher to call the event handler serially and avoid spawning
	// a goroutine per event which could lead to leaks if the handler is slow or blocked.
	if ms.onEvent != nil {
		ms.eventWg.Add(1)
		go func() {
			defer ms.eventWg.Done()
			for ev := range ms.eventCh {
				// Protect against panics in the handler
				func() {
					defer func() {
						_ = recover()
					}()
					ms.onEvent(ev)
				}()
			}
		}()
	}

	return ms
}

// Connect establishes connection to the server.
func (s *ManagedServer) Connect(ctx context.Context) error {
	s.mu.Lock()
	s.info.Status = StatusConnecting
	s.mu.Unlock()

	s.emitEvent(ServerEvent{
		Type:      EventConnected,
		Server:    s.config.Name,
		Status:    StatusConnecting,
		Timestamp: time.Now(),
	})

	if err := s.transport.Connect(ctx); err != nil {
		s.mu.Lock()
		s.info.Status = StatusUnhealthy
		s.info.LastError = err.Error()
		s.mu.Unlock()

		s.emitEvent(ServerEvent{
			Type:      EventError,
			Server:    s.config.Name,
			Status:    StatusUnhealthy,
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		return err
	}

	now := time.Now()
	s.mu.Lock()
	s.info.Status = StatusHealthy
	s.info.ConnectedAt = &now
	s.info.LastPing = &now
	s.mu.Unlock()

	// Discover server capabilities from initialize response
	s.discoverCapabilities(ctx)

	// Discover tools
	if err := s.discoverTools(ctx); err != nil {
		s.mu.Lock()
		s.info.LastError = fmt.Sprintf("discover tools: %v", err)
		s.mu.Unlock()
	}

	// Discover resources (best-effort, not all servers support this)
	s.discoverResources(ctx)

	// Discover prompts (best-effort)
	s.discoverPrompts(ctx)

	s.emitEvent(ServerEvent{
		Type:      EventConnected,
		Server:    s.config.Name,
		Status:    StatusHealthy,
		Timestamp: time.Now(),
	})

	// Start health monitoring if enabled
	if s.config.HealthCheck != nil && s.config.HealthCheck.Enabled {
		s.startHealthMonitor()
	}

	return nil
}

// discoverCapabilities queries server capabilities.
// In MCP, capabilities are returned in the initialize response, which is
// already handled by the transport. This method queries additional info.
func (s *ManagedServer) discoverCapabilities(ctx context.Context) {
	// The transport already performed initialize and may cache the result.
	// Query for server info if the server supports it.
	result, err := s.transport.Send(ctx, "server/info", nil)
	if err != nil {
		// Not all servers support this; rely on what transport captured.
		return
	}

	var info struct {
		Name         string          `json:"name"`
		Version      string          `json:"version"`
		Capabilities map[string]bool `json:"capabilities"`
	}

	if err := json.Unmarshal(result, &info); err == nil {
		s.mu.Lock()
		if info.Version != "" {
			s.info.Version = info.Version
		}
		if s.info.Capabilities == nil {
			s.info.Capabilities = make(map[string]bool)
		}
		for k, v := range info.Capabilities {
			s.info.Capabilities[k] = v
		}
		s.mu.Unlock()
	}
}

// discoverTools queries available tools from the server.
func (s *ManagedServer) discoverTools(ctx context.Context) error {
	result, err := s.transport.Send(ctx, "tools/list", nil)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	var response struct {
		Tools []struct {
			Name        string           `json:"name"`
			Description string           `json:"description"`
			InputSchema json.RawMessage  `json:"inputSchema"`
			Annotations *ToolAnnotations `json:"annotations,omitempty"`
		} `json:"tools"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("parse tools: %w", err)
	}

	tools := make([]ToolInfo, 0, len(response.Tools))
	for _, t := range response.Tools {
		qualifiedName := t.Name
		if s.config.Namespace != "" {
			qualifiedName = s.config.Namespace + "/" + t.Name
		}

		tools = append(tools, ToolInfo{
			Name:          t.Name,
			Description:   t.Description,
			InputSchema:   t.InputSchema,
			Server:        s.config.Name,
			QualifiedName: qualifiedName,
			Priority:      s.config.Priority,
			Annotations:   t.Annotations,
		})
	}

	s.mu.Lock()
	s.info.Tools = tools
	s.mu.Unlock()

	s.emitEvent(ServerEvent{
		Type:      EventToolsUpdated,
		Server:    s.config.Name,
		Message:   fmt.Sprintf("discovered %d tools", len(tools)),
		Timestamp: time.Now(),
	})

	return nil
}

// discoverResources queries available resources from the server (best-effort).
func (s *ManagedServer) discoverResources(ctx context.Context) {
	result, err := s.transport.Send(ctx, "resources/list", nil)
	if err != nil {
		return // Server may not support resources
	}

	var response struct {
		Resources []struct {
			URI         string `json:"uri"`
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			MimeType    string `json:"mimeType,omitempty"`
		} `json:"resources"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return
	}

	resources := make([]ResourceInfo, 0, len(response.Resources))
	for _, r := range response.Resources {
		resources = append(resources, ResourceInfo{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
			Server:      s.config.Name,
		})
	}

	s.mu.Lock()
	s.info.Resources = resources
	s.mu.Unlock()
}

// discoverPrompts queries available prompts from the server (best-effort).
func (s *ManagedServer) discoverPrompts(ctx context.Context) {
	result, err := s.transport.Send(ctx, "prompts/list", nil)
	if err != nil {
		return // Server may not support prompts
	}

	var response struct {
		Prompts []struct {
			Name        string           `json:"name"`
			Description string           `json:"description,omitempty"`
			Arguments   []PromptArgument `json:"arguments,omitempty"`
		} `json:"prompts"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return
	}

	prompts := make([]PromptInfo, 0, len(response.Prompts))
	for _, p := range response.Prompts {
		prompts = append(prompts, PromptInfo{
			Name:        p.Name,
			Description: p.Description,
			Arguments:   p.Arguments,
			Server:      s.config.Name,
		})
	}

	s.mu.Lock()
	s.info.Prompts = prompts
	s.mu.Unlock()
}

// startHealthMonitor starts periodic health checks.
func (s *ManagedServer) startHealthMonitor() {
	// If there's an existing monitor, stop it first to avoid dangling goroutines.
	var prevCancel context.CancelFunc
	var prevDone chan struct{}

	s.mu.Lock()
	prevCancel = s.healthCancel
	prevDone = s.healthDone
	s.mu.Unlock()

	if prevCancel != nil {
		prevCancel()
		if prevDone != nil {
			<-prevDone
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	s.mu.Lock()
	s.healthCancel = cancel
	s.healthDone = done
	s.mu.Unlock()

	interval := s.config.HealthCheck.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkHealth(ctx)
			}
		}
	}()
}

// checkHealth performs a health check.
func (s *ManagedServer) checkHealth(ctx context.Context) {
	timeout := s.config.HealthCheck.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	_, err := s.transport.Send(checkCtx, "ping", nil)
	latency := time.Since(start)

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.info.LastPing = &now
	s.info.Latency = latency

	oldStatus := s.info.Status

	if err != nil {
		s.info.Status = StatusUnhealthy
		s.info.LastError = err.Error()
	} else {
		s.info.Status = StatusHealthy
		s.info.LastError = ""
	}

	if oldStatus != s.info.Status {
		s.emitEvent(ServerEvent{
			Type:      EventHealthChange,
			Server:    s.config.Name,
			Status:    s.info.Status,
			Timestamp: time.Now(),
		})
	}
}

// Close disconnects from the server.
func (s *ManagedServer) Close() error {
	// Stop health monitoring
	if s.healthCancel != nil {
		s.healthCancel()
		<-s.healthDone
	}

	s.mu.Lock()
	s.info.Status = StatusDisconnected
	s.mu.Unlock()

	err := s.transport.Close()

	// Emit disconnected event
	s.emitEvent(ServerEvent{
		Type:      EventDisconnected,
		Server:    s.config.Name,
		Status:    StatusDisconnected,
		Timestamp: time.Now(),
	})

	// Close event channel and wait for dispatcher to finish
	s.mu.Lock()
	ch := s.eventCh
	s.eventCh = nil
	s.mu.Unlock()

	if ch != nil {
		close(ch)
		s.eventWg.Wait()
	}

	return err
}

// Call executes a method on the server.
func (s *ManagedServer) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	start := time.Now()
	s.requestCount.Add(1)

	result, err := s.transport.Send(ctx, method, params)
	latency := time.Since(start)

	s.totalLatency.Add(int64(latency))

	if err != nil {
		s.errorCount.Add(1)
		s.mu.Lock()
		s.info.LastError = err.Error()
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.info.RequestCount = s.requestCount.Load()
	s.info.ErrorCount = s.errorCount.Load()
	if s.info.RequestCount > 0 {
		s.info.Latency = time.Duration(s.totalLatency.Load() / s.info.RequestCount)
	}
	s.mu.Unlock()

	return result, err
}

// CallTool executes a tool on the server.
func (s *ManagedServer) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	start := time.Now()

	result, err := s.Call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})

	if err != nil {
		return nil, err
	}

	var response struct {
		Content []ContentBlock `json:"content"`
		IsError bool           `json:"isError,omitempty"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("parse tool result: %w", err)
	}

	return &ToolResult{
		Content: response.Content,
		IsError: response.IsError,
		Server:  s.config.Name,
		Latency: time.Since(start),
	}, nil
}

// Info returns current server information.
func (s *ManagedServer) Info() ServerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy
	info := *s.info
	info.Tools = make([]ToolInfo, len(s.info.Tools))
	copy(info.Tools, s.info.Tools)
	if len(s.info.Resources) > 0 {
		info.Resources = make([]ResourceInfo, len(s.info.Resources))
		copy(info.Resources, s.info.Resources)
	}
	if len(s.info.Prompts) > 0 {
		info.Prompts = make([]PromptInfo, len(s.info.Prompts))
		copy(info.Prompts, s.info.Prompts)
	}

	return info
}

// Config returns the server configuration.
func (s *ManagedServer) Config() *ServerConfig {
	return s.config
}

// IsHealthy returns true if the server is healthy.
func (s *ManagedServer) IsHealthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.info.Status == StatusHealthy
}

// Tools returns the list of available tools.
func (s *ManagedServer) Tools() []ToolInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]ToolInfo, len(s.info.Tools))
	copy(tools, s.info.Tools)
	return tools
}

// RefreshTools re-discovers tools, resources, and prompts from the server.
func (s *ManagedServer) RefreshTools(ctx context.Context) error {
	if err := s.discoverTools(ctx); err != nil {
		return err
	}
	s.discoverResources(ctx)
	s.discoverPrompts(ctx)
	return nil
}

// UpdateConfig updates the server configuration (for hot-reload).
func (s *ManagedServer) UpdateConfig(config *ServerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
	s.info.Config = config
}

func (s *ManagedServer) emitEvent(event ServerEvent) {
	// Non-blocking enqueue to the event dispatcher. Drop the event if the queue is full
	// or the dispatcher is not present to avoid goroutine leaks.
	s.mu.RLock()
	ch := s.eventCh
	s.mu.RUnlock()

	if ch == nil {
		// No dispatcher available; fall back to synchronous call if handler exists
		if s.onEvent != nil {
			// Protect against panics
			func() {
				defer func() { _ = recover() }()
				s.onEvent(event)
			}()
		}
		return
	}

	select {
	case ch <- event:
		// enqueued
	default:
		// queue full, drop to avoid blocking
	}
}
