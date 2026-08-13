package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// rpcResponse carries either a result or an error from the server.
type rpcResponse struct {
	data json.RawMessage
	err  error
}

// Client is a basic JSON-RPC LSP client.
type Client struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	nextID  atomic.Int64
	pending map[int64]chan rpcResponse

	// notify is invoked for server notifications (method, params).
	notify func(method string, params json.RawMessage)
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
		pending: make(map[int64]chan rpcResponse),
	}
	go c.readLoop()
	return c, nil
}

// SetNotificationHandler sets a function to be called for server notifications.
func (c *Client) SetNotificationHandler(fn func(method string, params json.RawMessage)) {
	c.mu.Lock()
	c.notify = fn
	c.mu.Unlock()
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
	ch := make(chan rpcResponse, 1)
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
		// clean up pending on send failure
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		// remove pending to avoid memory leak
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		return resp.data, resp.err
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

// failAllPending notifies all pending requests of a fatal read loop error and clears the map.
func (c *Client) failAllPending(err error) {
	c.mu.Lock()
	for id, ch := range c.pending {
		// best-effort non-blocking send into buffered channel
		select {
		case ch <- rpcResponse{nil, err}:
		default:
		}
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

func (c *Client) readLoop() {
	reader := bufio.NewReader(c.stdout)
	for {
		// 1. Read headers
		var contentLength int
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				// notify pending callers and exit
				c.failAllPending(err)
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break // End of headers
			}
			if strings.HasPrefix(line, "Content-Length:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					n, perr := strconv.Atoi(strings.TrimSpace(parts[1]))
					if perr != nil || n <= 0 {
						c.failAllPending(fmt.Errorf("invalid Content-Length header: %q", parts[1]))
						return
					}
					contentLength = n
				}
			}
		}

		if contentLength <= 0 {
			// nothing to read
			continue
		}

		// 2. Read body
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(reader, body); err != nil {
			c.failAllPending(err)
			return
		}

		// 3. Decode message (notifications or responses)
		var msg struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int             `json:"code"`
				Message string          `json:"message"`
				Data    json.RawMessage `json:"data,omitempty"`
			} `json:"error"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &msg); err != nil {
			// malformed message; skip to next
			continue
		}

		// Notification
		if msg.ID == nil {
			if c.notify != nil && msg.Method != "" {
				// dispatch asynchronously so readLoop isn't blocked
				go c.notify(msg.Method, msg.Params)
			}
			continue
		}

		// Response to a request
		c.mu.Lock()
		ch, ok := c.pending[*msg.ID]
		if ok {
			delete(c.pending, *msg.ID)
		}
		c.mu.Unlock()
		if !ok {
			// no waiter; drop response
			continue
		}

		if msg.Error != nil {
			// send error back to caller
			select {
			case ch <- rpcResponse{nil, errors.New(msg.Error.Message)}:
			default:
			}
			continue
		}

		// normal result
		select {
		case ch <- rpcResponse{msg.Result, nil}:
		default:
		}
	}
}
