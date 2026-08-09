package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

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
	url, _ := args["url"].(string)
	if url == "" {
		return tools.Result{IsError: true, Content: "url is required"}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	return tools.Result{Content: string(body)}, nil
}
