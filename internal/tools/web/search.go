package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return tools.Result{IsError: true, Content: fmt.Sprintf("search HTTP error: %s", resp.Status)}, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	return tools.Result{Content: summarizeDuckDuckGoHTML(query, string(body))}, nil
}

func summarizeDuckDuckGoHTML(query, html string) string {
	re := regexp.MustCompile(`(?s)<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(html, 6)
	if len(matches) == 0 {
		return fmt.Sprintf("no parsed results for query %q", query)
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("Top results for %q:", query))
	for i, m := range matches {
		link := strings.TrimSpace(htmlToText(m[1]))
		title := strings.TrimSpace(htmlToText(m[2]))
		if title == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, title, link))
	}
	if len(lines) == 1 {
		return fmt.Sprintf("no parsed results for query %q", query)
	}
	return strings.Join(lines, "\n")
}

func htmlToText(s string) string {
	tagRe := regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRe := regexp.MustCompile(`\s+`)
	out := tagRe.ReplaceAllString(s, " ")
	out = strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
	).Replace(out)
	return strings.TrimSpace(spaceRe.ReplaceAllString(out, " "))
}
