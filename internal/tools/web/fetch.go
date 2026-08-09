package web

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	tools "github.com/iSundram/Automergent/internal/tools"
)

// blockedHostnames are cloud-metadata and other dangerous endpoints that must
// always be blocked regardless of IP-range checks.
var blockedHostnames = []string{
	"169.254.169.254",          // AWS/Azure/GCP instance metadata
	"metadata.google.internal", // GCP metadata
	"instance-data",            // Various
}

// validateURL enforces SSRF protections: only http/https, and no private/local IPs.
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https URLs are allowed (got %q)", u.Scheme)
	}
	hostname := u.Hostname()
	for _, blocked := range blockedHostnames {
		if hostname == blocked {
			return fmt.Errorf("blocked host: %s", hostname)
		}
	}
	// Resolve hostname and reject private/local IPs.
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		// DNS failure is surfaced as a fetch error rather than a validation error.
		return nil
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("fetching private/local addresses is blocked (resolved %s to %s)", hostname, addr)
		}
	}
	return nil
}

// FetchTool fetches the content of a URL.
type FetchTool struct {
	client *http.Client
}

func NewFetchTool() *FetchTool {
	return &FetchTool{client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *FetchTool) Name() string                          { return "web_fetch" }
func (t *FetchTool) Description() string                   { return "Fetch the content of a web URL." }
func (t *FetchTool) RequiresConfirmation(mode string) bool { return false }

func (t *FetchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string", "description": "URL to fetch."},
		},
		"required": []string{"url"},
	}
}

func (t *FetchTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	rawURL, ok := tools.StringArg(args, "url")
	if !ok || rawURL == "" {
		return tools.Result{IsError: true, Content: "url is required"}, nil
	}
	if err := validateURL(rawURL); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("url blocked: %v", err)}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("invalid url: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "Automergent/0.1.0")
	resp, err := t.client.Do(req)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("fetch error: %v", err)}, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read error: %v", err)}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("HTTP %s\n%s", resp.Status, string(body)),
		}, nil
	}
	return tools.Result{Content: string(body)}, nil
}
