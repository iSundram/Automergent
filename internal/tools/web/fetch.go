package web

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	tools "github.com/iSundram/Automergent/internal/tools"
)

// blockedHostnames are cloud-metadata and other dangerous endpoints that must
// always be blocked regardless of IP-range checks.
var blockedHostnames = []string{
	"169.254.169.254",          // AWS/Azure/GCP instance metadata
	"metadata.google.internal", // GCP metadata
	"instance-data",            // Various
	"localhost",                // Always block literal localhost
}

// context key for resolved host map
type resolvedKeyType struct{}

// normalizeHostname trims a trailing dot and lowercases the hostname to prevent
// trivial DNS rebinding bypasses such as "localhost.".
func normalizeHostname(h string) string {
	h = strings.ToLower(strings.TrimSuffix(h, "."))
	return h
}

// parseNumericIP attempts to parse non-standard numeric IP representations
// such as hex (0x7f000001), decimal (2130706433), octal (0177.0.0.1), or
// dotted octal/hex segments. Returns nil if parsing fails.
func parseNumericIP(host string) net.IP {
	// First, try the normal parser which handles dotted decimal and IPv6.
	if ip := net.ParseIP(host); ip != nil {
		return ip
	}
	// If host contains dots, try to parse four segments where each segment
	// can be decimal, hex (0x...), or octal (leading 0).
	if strings.Contains(host, ".") {
		parts := strings.Split(host, ".")
		if len(parts) == 4 {
			bytes := make([]byte, 4)
			for i, p := range parts {
				// base 0 lets ParseUint interpret 0x... as hex and leading 0 as octal.
				v, err := strconv.ParseUint(p, 0, 8)
				if err != nil || v > 255 {
					return nil
				}
				bytes[i] = byte(v)
			}
			return net.IPv4(bytes[0], bytes[1], bytes[2], bytes[3])
		}
		// fallthrough: other dotted formats not supported
	}
	// No dots: try single integer representation (decimal/hex/octal)
	if v, err := strconv.ParseUint(host, 0, 32); err == nil {
		b0 := byte((v >> 24) & 0xFF)
		b1 := byte((v >> 16) & 0xFF)
		b2 := byte((v >> 8) & 0xFF)
		b3 := byte(v & 0xFF)
		return net.IPv4(b0, b1, b2, b3)
	}
	return nil
}

// isBlockedIP returns true if the IP is loopback, private, link-local, or
// otherwise reserved for internal networks.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// Block the well-known cloud metadata address explicitly.
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return true
	}
	// IPv6 unique local addresses (fc00::/7)
	if ip.To16() != nil && ip.To4() == nil {
		// check fc00::/7
		first := ip[0]
		if first&0xfe == 0xfc { // fc00/7
			return true
		}
	}
	return false
}

// resolveAndValidate resolves the hostname once and validates all returned
// IPs are not internal. Returns the resolved IPs or an error.
func resolveAndValidate(hostname string) ([]net.IP, error) {
	n := normalizeHostname(hostname)
	// Block explicit blocked hostnames immediately.
	for _, b := range blockedHostnames {
		if n == b {
			return nil, fmt.Errorf("blocked host: %s", n)
		}
	}
	// If hostname is a numeric representation, parse it directly.
	if ip := parseNumericIP(n); ip != nil {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("fetching private/local addresses is blocked (resolved %s to %s)", hostname, ip.String())
		}
		return []net.IP{ip}, nil
	}
	// Perform DNS resolution once.
	ips, err := net.LookupIP(n)
	if err != nil {
		return nil, fmt.Errorf("dns lookup failed for %s: %w", n, err)
	}
	valid := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("fetching private/local addresses is blocked (resolved %s to %s)", hostname, ip.String())
		}
		valid = append(valid, ip)
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("no valid IPs resolved for %s", hostname)
	}
	return valid, nil
}

// validateURL remains for tests and external callers. It mirrors the legacy
// behavior where DNS lookup failures are surfaced later during fetch instead
// of as validation failures.
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https URLs are allowed (got %q)", u.Scheme)
	}
	// Try resolving; if DNS fails, treat it as non-validation error so callers
	// can surface a fetch-time error instead.
	if _, err := resolveAndValidate(u.Hostname()); err != nil {
		if strings.Contains(err.Error(), "dns lookup failed for") {
			return nil
		}
		return err
	}
	return nil
}

// validateContentType ensures the response Content-Type is an expected, safe
// type. We allow text/* and a small set of application mime types.
func validateContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/javascript", "application/xml", "application/xhtml+xml":
		return true
	default:
		return false
	}
}

// FetchTool fetches the content of a URL with strong SSRF protections.
type FetchTool struct {
}

func NewFetchTool() *FetchTool {
	return &FetchTool{}
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
	// Parse URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("invalid url: %v", err)}, nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return tools.Result{IsError: true, Content: fmt.Sprintf("only http/https URLs are allowed (got %q)", u.Scheme)}, nil
	}
	origHost := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	// Resolve and validate the original hostname once.
	origIPs, err := resolveAndValidate(origHost)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("url blocked: %v", err)}, nil
	}
	// Prepare a per-request resolved-host map stored in the context so the
	// custom DialContext can use the exact IPs we validated above for dialing.
	resolvedHosts := map[string]net.IP{}
	resolvedHosts[normalizeHostname(origHost)] = origIPs[0]
	ctx = context.WithValue(ctx, resolvedKeyType{}, resolvedHosts)

	// Custom DialContext that uses the resolvedHosts map from the request's
	// context to dial the exact IP we validated above. Falls back to the
	// default dialer if no mapping exists.
	dialer := &net.Dialer{}
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, p, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if v := ctx.Value(resolvedKeyType{}); v != nil {
			if m, ok := v.(map[string]net.IP); ok {
				if ip, found := m[normalizeHostname(host)]; found {
					return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), p))
				}
			}
		}
		return dialer.DialContext(ctx, network, addr)
	}

	transport := &http.Transport{
		DialContext:         dialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   true,
		ForceAttemptHTTP2:   false,
	}

	// Track redirect count and validate each redirect target before following.
	maxRedirects := 10
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			// Validate the redirect target and add its resolved IP to the map so
			// our DialContext will use the validated IP for the subsequent request.
			rh := req.URL.Hostname()
			ips, err := resolveAndValidate(rh)
			if err != nil {
				return err
			}
			// Propagate the resolved map into the new request's context.
			if v := req.Context().Value(resolvedKeyType{}); v != nil {
				if m, ok := v.(map[string]net.IP); ok {
					m[normalizeHostname(rh)] = ips[0]
					req = req.WithContext(context.WithValue(req.Context(), resolvedKeyType{}, m))
				}
			}
			return nil
		},
		Timeout: 15 * time.Second,
	}

	// Create request with the augmented context that contains our resolved map.
	reqCtx := ctx
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("invalid url: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "Automergent/0.1.0")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("fetch error: %v", err)}, nil
	}
	defer resp.Body.Close()

	// Validate content type
	if !validateContentType(resp.Header.Get("Content-Type")) {
		return tools.Result{IsError: true, Content: fmt.Sprintf("disallowed content-type: %q", resp.Header.Get("Content-Type"))}, nil
	}

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

// EstimatedCost returns cost estimates for the fetch tool.
func (t *FetchTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 500, LatencyMs: 1000, RiskLevel: "low"}
}
