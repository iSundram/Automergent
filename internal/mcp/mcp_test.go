package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/config"
)

func TestFromAppConfig(t *testing.T) {
	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"test-server": {
				Type:    "http",
				URL:     "http://localhost:8080",
				Command: []string{"echo"},
				Env:     map[string]string{"KEY": "val"},
				Auth:    "token123",
				Timeout: "5s",
			},
		},
	}

	orchCfg := FromAppConfig(cfg)
	if len(orchCfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(orchCfg.Servers))
	}

	srv := orchCfg.Servers[0]
	if srv.Name != "test-server" {
		t.Errorf("expected name 'test-server', got %q", srv.Name)
	}
	if srv.Transport != TransportHTTP {
		t.Errorf("expected transport HTTP, got %q", srv.Transport)
	}
	if srv.URL != "http://localhost:8080" {
		t.Errorf("expected URL 'http://localhost:8080', got %q", srv.URL)
	}
	if srv.AuthToken != "token123" {
		t.Errorf("expected auth token 'token123', got %q", srv.AuthToken)
	}
	if srv.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", srv.Timeout)
	}
	if !srv.Enabled {
		t.Error("expected server to be enabled")
	}
}

func TestFromAppConfigStdio(t *testing.T) {
	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"stdio-srv": {
				Type:    "stdio",
				Command: []string{"mcp-server", "--port", "3000"},
			},
		},
	}

	orchCfg := FromAppConfig(cfg)
	srv := orchCfg.Servers[0]
	if srv.Transport != TransportStdio {
		t.Errorf("expected stdio transport, got %q", srv.Transport)
	}
	if srv.Command != "mcp-server" {
		t.Errorf("expected command 'mcp-server', got %q", srv.Command)
	}
	if len(srv.Args) != 2 || srv.Args[0] != "--port" || srv.Args[1] != "3000" {
		t.Errorf("expected args [--port 3000], got %v", srv.Args)
	}
}

func TestRedactSecret(t *testing.T) {
	cfg := &ServerConfig{
		Name:      "test",
		AuthToken: "super-secret-token",
	}

	redacted := RedactSecret(cfg)
	if redacted.AuthToken != "***" {
		t.Errorf("expected redacted token '***', got %q", redacted.AuthToken)
	}
	if redacted.Name != "test" {
		t.Errorf("expected name unchanged, got %q", redacted.Name)
	}
	// Original should be unchanged
	if cfg.AuthToken != "super-secret-token" {
		t.Error("original config was modified")
	}
}

func TestValidateServerConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ServerConfig
		wantErr bool
	}{
		{
			name:    "valid http",
			cfg:     &ServerConfig{Name: "s1", Transport: TransportHTTP, URL: "http://localhost"},
			wantErr: false,
		},
		{
			name:    "valid stdio",
			cfg:     &ServerConfig{Name: "s2", Transport: TransportStdio, Command: "echo"},
			wantErr: false,
		},
		{
			name:    "missing name",
			cfg:     &ServerConfig{Transport: TransportHTTP, URL: "http://localhost"},
			wantErr: true,
		},
		{
			name:    "http without url",
			cfg:     &ServerConfig{Name: "s3", Transport: TransportHTTP},
			wantErr: true,
		},
		{
			name:    "stdio without command",
			cfg:     &ServerConfig{Name: "s4", Transport: TransportStdio},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateServerConfig(tt.cfg)
			if tt.wantErr && len(errs) == 0 {
				t.Error("expected validation errors, got none")
			}
			if !tt.wantErr && len(errs) > 0 {
				t.Errorf("unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestOrchestratorToolIndex(t *testing.T) {
	orch := NewOrchestrator(OrchestratorConfig{})
	defer orch.Stop()

	// Empty orchestrator should have no tools
	tools := orch.ListTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestOrchestratorServerList(t *testing.T) {
	orch := NewOrchestrator(OrchestratorConfig{})
	defer orch.Stop()

	servers := orch.ListServers()
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

func TestResponseCache(t *testing.T) {
	cache := newResponseCache(time.Second)

	// Empty cache
	_, ok := cache.get("key1")
	if ok {
		t.Error("expected cache miss")
	}

	// Set and get
	cache.set("key1", &ToolResult{Content: []ContentBlock{{Type: "text", Text: "hello"}}})
	result, ok := cache.get("key1")
	if !ok {
		t.Error("expected cache hit")
	}
	if result.Content[0].Text != "hello" {
		t.Errorf("expected 'hello', got %q", result.Content[0].Text)
	}

	// Invalidate
	cache.invalidate("key1")
	_, ok = cache.get("key1")
	if ok {
		t.Error("expected cache miss after invalidation")
	}
}

func TestResponseCacheExpiry(t *testing.T) {
	cache := newResponseCache(50 * time.Millisecond)

	cache.set("key1", &ToolResult{Content: []ContentBlock{{Type: "text", Text: "hello"}}})

	// Should exist immediately
	_, ok := cache.get("key1")
	if !ok {
		t.Error("expected cache hit before expiry")
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	_, ok = cache.get("key1")
	if ok {
		t.Error("expected cache miss after expiry")
	}
}

func TestToolAnnotations(t *testing.T) {
	info := ToolInfo{
		Name:          "test-tool",
		QualifiedName: "test-tool",
		Annotations: &ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: false,
		},
	}

	if !info.Annotations.ReadOnlyHint {
		t.Error("expected ReadOnlyHint to be true")
	}
	if info.Annotations.DestructiveHint {
		t.Error("expected DestructiveHint to be false")
	}
}

func TestProtocolVersions(t *testing.T) {
	if ProtocolVersion20241105 != "2024-11-05" {
		t.Errorf("unexpected protocol version: %s", ProtocolVersion20241105)
	}
	if ProtocolVersion20250326 != "2025-03-26" {
		t.Errorf("unexpected protocol version: %s", ProtocolVersion20250326)
	}
}

func TestCreateTransport(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ServerConfig
		want TransportType
	}{
		{
			name: "http",
			cfg:  &ServerConfig{Transport: TransportHTTP, URL: "http://localhost"},
			want: TransportHTTP,
		},
		{
			name: "stdio",
			cfg:  &ServerConfig{Transport: TransportStdio, Command: "echo"},
			want: TransportStdio,
		},
		{
			name: "sse",
			cfg:  &ServerConfig{Transport: TransportSSE, URL: "http://localhost"},
			want: TransportSSE,
		},
		{
			name: "default-http",
			cfg:  &ServerConfig{URL: "http://localhost"},
			want: TransportHTTP,
		},
		{
			name: "default-stdio",
			cfg:  &ServerConfig{Command: "echo"},
			want: TransportStdio,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := CreateTransport(tt.cfg)
			if tr == nil {
				t.Fatal("expected non-nil transport")
			}
			_ = context.Background() // Ensure context is available
		})
	}
}
