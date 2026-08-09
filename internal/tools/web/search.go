package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// SearchTool performs a web search via DuckDuckGo HTML.
type SearchTool struct {
	client *http.Client
}

func NewSearchTool() *SearchTool {
	return &SearchTool{client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *SearchTool) Name() string                          { return "web_search" }
func (t *SearchTool) Description() string                   { return "Search the web for a query." }
func (t *SearchTool) RequiresConfirmation(mode string) bool { return false }

func (t *SearchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query."},
		},
		"required": []string{"query"},
	}
}

func (t *SearchTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return tools.Result{IsError: true, Content: "query is required"}, nil
	}
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("request error: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "Automergent/0.1.0")
	resp, err := t.client.Do(req)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("search error: %v", err)}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	return tools.Result{Content: string(body)}, nil
}
