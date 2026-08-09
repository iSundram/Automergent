package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client talks to an MCP (Model Context Protocol) server.
type Client struct {
	httpClient *http.Client
	baseURL    string
	authToken  string
}

// NewHTTPClient creates an MCP client that communicates over HTTP.
func NewHTTPClient(baseURL, authToken string) *Client {
	return &Client{
		httpClient: &http.Client{},
		baseURL:    baseURL,
		authToken:  authToken,
	}
}

type mcpRequest struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type mcpResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *mcpError       `json:"error"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Call sends an MCP request and returns the raw result.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(mcpRequest{Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	if c.authToken != "" {
		req.Header.Set("authorization", "Bearer "+c.authToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcp: status %d: %s", resp.StatusCode, data)
	}
	var mcpResp mcpResponse
	if err := json.Unmarshal(data, &mcpResp); err != nil {
		return nil, err
	}
	if mcpResp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}
	return mcpResp.Result, nil
}
