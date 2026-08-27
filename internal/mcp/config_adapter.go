package mcp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/config"
)

// FromAppConfig converts application config MCP servers to the orchestrator config.
func FromAppConfig(cfg config.MCPConfig) OrchestratorConfig {
	servers := make([]ServerConfig, 0, len(cfg.Servers))
	for name, srv := range cfg.Servers {
		sc := ServerConfig{
			Name:    name,
			Enabled: true,
		}
		// Map transport type
		switch strings.ToLower(srv.Type) {
		case "stdio":
			sc.Transport = TransportStdio
		case "sse":
			sc.Transport = TransportSSE
		case "http", "streamable-http", "":
			sc.Transport = TransportHTTP
		default:
			sc.Transport = TransportHTTP
		}

		// Map command: first element is the command, rest are args
		if len(srv.Command) > 0 {
			sc.Command = srv.Command[0]
			if len(srv.Command) > 1 {
				sc.Args = srv.Command[1:]
			}
		}
		sc.URL = srv.URL
		sc.Env = srv.Env
		sc.AuthToken = srv.Auth

		// Parse timeout
		if srv.Timeout != "" {
			if d, err := time.ParseDuration(srv.Timeout); err == nil {
				sc.Timeout = d
			}
		}
		if sc.Timeout == 0 {
			sc.Timeout = 30 * time.Second
		}
		sc.RetryCount = 3

		servers = append(servers, sc)
	}

	return OrchestratorConfig{
		Servers:         servers,
		DefaultTimeout:  30 * time.Second,
		MaxConcurrent:   10,
		LoadBalancing:   LoadBalancePriority,
		FailoverEnabled: true,
		CacheEnabled:    false,
	}
}

// ServerConfigFromApp converts a single named MCP server from app config.
func ServerConfigFromApp(name string, srv config.MCPServer) ServerConfig {
	return FromAppConfig(config.MCPConfig{
		Servers: map[string]config.MCPServer{name: srv},
	}).Servers[0]
}

// RedactSecret returns a copy of the config with sensitive fields masked.
func RedactSecret(cfg *ServerConfig) *ServerConfig {
	if cfg == nil {
		return nil
	}
	out := *cfg
	if out.AuthToken != "" {
		out.AuthToken = "***"
	}
	return &out
}

// ValidateServerConfig checks a server config for common issues.
func ValidateServerConfig(cfg *ServerConfig) []string {
	var errs []string
	if cfg.Name == "" {
		errs = append(errs, "server name is required")
	}
	if cfg.Transport == TransportHTTP || cfg.Transport == TransportSSE {
		if cfg.URL == "" {
			errs = append(errs, fmt.Sprintf("server %q: url is required for %s transport", cfg.Name, cfg.Transport))
		}
	}
	if cfg.Transport == TransportStdio {
		if cfg.Command == "" {
			errs = append(errs, fmt.Sprintf("server %q: command is required for stdio transport", cfg.Name))
		}
	}
	if cfg.Timeout < 0 {
		errs = append(errs, fmt.Sprintf("server %q: timeout must be non-negative", cfg.Name))
	}
	return errs
}

// DurationToString converts a duration to a human-readable string.
func DurationToString(d time.Duration) string {
	if d < time.Second {
		return strconv.Itoa(int(d.Milliseconds())) + "ms"
	}
	if d < time.Minute {
		return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
	}
	return strconv.FormatFloat(d.Minutes(), 'f', 1, 64) + "m"
}
