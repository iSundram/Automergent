package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// httpTransport implements Transport over HTTP.
type httpTransport struct {
	client    *http.Client
	baseURL   string
	authToken string
	connected atomic.Bool
	mu        sync.Mutex
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(baseURL, authToken string, timeout time.Duration) Transport {
	return &httpTransport{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL:   baseURL,
		authToken: authToken,
	}
}

func (t *httpTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Test connection with initialize
	_, err := t.Send(ctx, "ping", nil)
	if err != nil {
		// Ping may not be supported, try initialize
		_, err = t.Send(ctx, "initialize", map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "automergent",
				"version": "1.0.0",
			},
		})
		if err != nil {
			return fmt.Errorf("http transport connect: %w", err)
		}
	}
	t.connected.Store(true)
	return nil
}

func (t *httpTransport) Close() error {
	t.connected.Store(false)
	return nil
}

func (t *httpTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if t.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.authToken)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		t.connected.Store(false)
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, string(data))
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func (t *httpTransport) IsConnected() bool {
	return t.connected.Load()
}

// stdioTransport implements Transport over stdin/stdout of a subprocess.
type stdioTransport struct {
	command   string
	args      []string
	env       map[string]string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	mu        sync.Mutex
	connected atomic.Bool
	reqID     atomic.Int64
	responses map[int64]chan json.RawMessage
	respMu    sync.Mutex
	done      chan struct{}
}

// NewStdioTransport creates a new stdio transport.
func NewStdioTransport(command string, args []string, env map[string]string) Transport {
	return &stdioTransport{
		command:   command,
		args:      args,
		env:       env,
		responses: make(map[int64]chan json.RawMessage),
		done:      make(chan struct{}),
	}
}

func (t *stdioTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cmd = exec.CommandContext(ctx, t.command, t.args...)

	// Set up environment
	t.cmd.Env = os.Environ()
	for k, v := range t.env {
		t.cmd.Env = append(t.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	t.stdout = bufio.NewReader(stdout)

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	// Start response reader
	go t.readResponses()

	t.connected.Store(true)

	// Initialize the connection
	_, err = t.Send(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "automergent",
			"version": "1.0.0",
		},
	})
	if err != nil {
		t.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	// Send initialized notification
	t.sendNotification("notifications/initialized", nil)

	return nil
}

func (t *stdioTransport) sendNotification(method string, params any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}

	data, _ := json.Marshal(msg)
	t.mu.Lock()
	t.stdin.Write(data)
	t.stdin.Write([]byte("\n"))
	t.mu.Unlock()
}

func (t *stdioTransport) readResponses() {
	for {
		select {
		case <-t.done:
			return
		default:
		}

		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			// Propagate connection closure to pending requests
			if err != io.EOF {
				t.connected.Store(false)
			}

			// Notify all pending response channels about the error so callers don't hang.
			t.respMu.Lock()
			for id, ch := range t.responses {
				select {
				case ch <- nil:
				default:
				}
				delete(t.responses, id)
			}
			t.respMu.Unlock()

			return
		}

		var msg struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(line, &msg); err != nil {
			continue // Skip malformed messages
		}

		t.respMu.Lock()
		if ch, ok := t.responses[msg.ID]; ok {
			// Prefer non-blocking send to avoid goroutine stalls if caller timed out and removed the channel
			select {
			case ch <- func() json.RawMessage {
				if msg.Error != nil {
					return nil
				}
				return msg.Result
			}():
			default:
			}
			delete(t.responses, msg.ID)
		}
		t.respMu.Unlock()
	}
}

func (t *stdioTransport) Close() error {
	t.connected.Store(false)
	close(t.done)

	if t.stdin != nil {
		t.stdin.Close()
	}

	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}

	return nil
}

func (t *stdioTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if !t.connected.Load() && method != "initialize" {
		return nil, fmt.Errorf("not connected")
	}

	id := t.reqID.Add(1)
	respCh := make(chan json.RawMessage, 1)

	t.respMu.Lock()
	t.responses[id] = respCh
	t.respMu.Unlock()

	defer func() {
		t.respMu.Lock()
		delete(t.responses, id)
		t.respMu.Unlock()
	}()

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	t.mu.Lock()
	_, err = t.stdin.Write(append(data, '\n'))
	t.mu.Unlock()

	if err != nil {
		t.connected.Store(false)
		return nil, fmt.Errorf("write: %w", err)
	}

	select {
	case result := <-respCh:
		if result == nil {
			return nil, fmt.Errorf("server returned error")
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *stdioTransport) IsConnected() bool {
	return t.connected.Load()
}

// sseTransport implements Transport using Server-Sent Events.
type sseTransport struct {
	baseURL   string
	authToken string
	client    *http.Client
	connected atomic.Bool
	mu        sync.Mutex
}

// NewSSETransport creates a new SSE transport.
func NewSSETransport(baseURL, authToken string, timeout time.Duration) Transport {
	return &sseTransport{
		baseURL:   baseURL,
		authToken: authToken,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (t *sseTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// SSE typically uses HTTP POST for requests and SSE for responses
	// For simplicity, we use HTTP transport semantics with SSE endpoint
	_, err := t.Send(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "automergent",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("sse connect: %w", err)
	}

	t.connected.Store(true)
	return nil
}

func (t *sseTransport) Close() error {
	t.connected.Store(false)
	return nil
}

func (t *sseTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	// SSE transport uses same HTTP request/response pattern
	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if t.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.authToken)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(data))
	}

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func (t *sseTransport) IsConnected() bool {
	return t.connected.Load()
}

// CreateTransport creates a transport based on the server configuration.
func CreateTransport(cfg *ServerConfig) Transport {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	switch cfg.Transport {
	case TransportHTTP:
		return NewHTTPTransport(cfg.URL, cfg.AuthToken, timeout)
	case TransportStdio:
		return NewStdioTransport(cfg.Command, cfg.Args, cfg.Env)
	case TransportSSE:
		return NewSSETransport(cfg.URL, cfg.AuthToken, timeout)
	default:
		// Default to HTTP if URL is provided, otherwise stdio
		if cfg.URL != "" {
			return NewHTTPTransport(cfg.URL, cfg.AuthToken, timeout)
		}
		return NewStdioTransport(cfg.Command, cfg.Args, cfg.Env)
	}
}
