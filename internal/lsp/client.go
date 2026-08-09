package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client is a basic JSON-RPC LSP client.
type Client struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	nextID  atomic.Int64
	pending map[int64]chan json.RawMessage
}

// Start launches an LSP server process.
func Start(ctx context.Context, command string, args ...string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp: start %q: %w", command, err)
	}
	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		pending: make(map[int64]chan json.RawMessage),
	}
	go c.readLoop()
	return c, nil
}

// Notify sends a notification (no response expected).
func (c *Client) Notify(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	return c.send(msg)
}

// Call sends a request and waits for a response.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if err := c.send(msg); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		return resp, nil
	}
}

// Close shuts down the LSP server.
func (c *Client) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}

func (c *Client) send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(data)
	return err
}

func (c *Client) readLoop() {
	dec := json.NewDecoder(c.stdout)
	for {
		var msg struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if err := dec.Decode(&msg); err != nil {
			return
		}
		if msg.ID == nil {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[*msg.ID]
		if ok {
			delete(c.pending, *msg.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- msg.Result
		}
	}
}
