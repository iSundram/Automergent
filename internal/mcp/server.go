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
}

// NewManagedServer creates a new managed server instance.
func NewManagedServer(config *ServerConfig, onEvent EventHandler) *ManagedServer {
	return &ManagedServer{
		config:     config,
		transport:  CreateTransport(config),
		onEvent:    onEvent,
		healthDone: make(chan struct{}),
		info: &ServerInfo{
			Config:       config,
			Status:       StatusDisconnected,
			Tools:        []ToolInfo{},
			Capabilities: make(map[string]bool),
		},
	}
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

	// Discover server capabilities and tools
	if err := s.discoverCapabilities(ctx); err != nil {
		s.mu.Lock()
		s.info.LastError = fmt.Sprintf("discover capabilities: %v", err)
		s.mu.Unlock()
	}

	if err := s.discoverTools(ctx); err != nil {
		s.mu.Lock()
		s.info.LastError = fmt.Sprintf("discover tools: %v", err)
		s.mu.Unlock()
	}

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
func (s *ManagedServer) discoverCapabilities(ctx context.Context) error {
	// Server info from initialize response is typically cached
	// We query for any additional capabilities
	result, err := s.transport.Send(ctx, "server/info", nil)
	if err != nil {
		// Some servers may not support this
		return nil
	}

	var info struct {
		Name         string          `json:"name"`
		Version      string          `json:"version"`
		Capabilities map[string]bool `json:"capabilities"`
	}

	if err := json.Unmarshal(result, &info); err == nil {
		s.mu.Lock()
		s.info.Version = info.Version
		s.info.Capabilities = info.Capabilities
		s.mu.Unlock()
	}

	return nil
}

// discoverTools queries available tools from the server.
func (s *ManagedServer) discoverTools(ctx context.Context) error {
	result, err := s.transport.Send(ctx, "tools/list", nil)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}

	var response struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
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

// startHealthMonitor starts periodic health checks.
func (s *ManagedServer) startHealthMonitor() {
	ctx, cancel := context.WithCancel(context.Background())
	s.healthCancel = cancel
	s.healthDone = make(chan struct{})

	interval := s.config.HealthCheck.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}

	go func() {
		defer close(s.healthDone)
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

	s.emitEvent(ServerEvent{
		Type:      EventDisconnected,
		Server:    s.config.Name,
		Status:    StatusDisconnected,
		Timestamp: time.Now(),
	})

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

// RefreshTools re-discovers tools from the server.
func (s *ManagedServer) RefreshTools(ctx context.Context) error {
	return s.discoverTools(ctx)
}

// UpdateConfig updates the server configuration (for hot-reload).
func (s *ManagedServer) UpdateConfig(config *ServerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
	s.info.Config = config
}

func (s *ManagedServer) emitEvent(event ServerEvent) {
	if s.onEvent != nil {
		go s.onEvent(event)
	}
}
