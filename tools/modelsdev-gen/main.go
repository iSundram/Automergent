// Command modelsdev-gen regenerates internal/modelsdev/snapshot.go from the
// live models.dev catalog. It keeps the embedded fallback from rotting:
//
//	go run ./tools/modelsdev-gen -o internal/modelsdev/snapshot.go
//
// Only tool-call capable models with at least modelsdev.MinContextTokens of
// context are embedded, matching the runtime listing filter.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/modelsdev"
)

// providers embedded in the snapshot, in output order.
var providers = []string{"anthropic", "openai", "google"}

const sourceURL = "https://models.dev/api.json"

type catalogModel struct {
	Name        string `json:"name"`
	Reasoning   bool   `json:"reasoning"`
	Attachment  bool   `json:"attachment"`
	ToolCall    bool   `json:"tool_call"`
	Knowledge   string `json:"knowledge"`
	ReleaseDate string `json:"release_date"`
	Limit       struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
	Cost struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cache_read"`
		CacheWrite float64 `json:"cache_write"`
	} `json:"cost"`
	ReasoningOptions []struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
	} `json:"reasoning_options"`
}

// effortValues mirrors modelsdev.effortValues (unexported there): the
// "effort"-typed reasoning option's values.
func effortValues(m catalogModel) []string {
	for _, opt := range m.ReasoningOptions {
		if opt.Type == "effort" && len(opt.Values) > 0 {
			return opt.Values
		}
	}
	return nil
}

func main() {
	out := flag.String("o", "internal/modelsdev/snapshot.go", "output file")
	flag.Parse()

	data := fetch()
	var catalog map[string]struct {
		Models map[string]catalogModel `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		fatal("parse catalog: %v", err)
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "package modelsdev\n\n")
	fmt.Fprintf(&b, "import \"github.com/iSundram/Automergent/internal/ai\"\n\n")
	fmt.Fprintf(&b, "// snapshotCatalog is the embedded fallback catalog, generated from\n")
	fmt.Fprintf(&b, "// %s by tools/modelsdev-gen (tool-call capable\n", sourceURL)
	fmt.Fprintf(&b, "// models with >= 1M context only). It is the last resort when neither the\n")
	fmt.Fprintf(&b, "// disk cache nor the network is available. Regenerate with:\n")
	fmt.Fprintf(&b, "//\n")
	fmt.Fprintf(&b, "//\tgo run ./tools/modelsdev-gen -o internal/modelsdev/snapshot.go\n")
	fmt.Fprintf(&b, "var snapshotCatalog = map[string][]ai.Model{\n")

	total := 0
	for _, slug := range providers {
		prov, ok := catalog[slug]
		if !ok || len(prov.Models) == 0 {
			fmt.Fprintf(os.Stderr, "warning: provider %q missing from catalog\n", slug)
			continue
		}
		ids := make([]string, 0, len(prov.Models))
		for id := range prov.Models {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		fmt.Fprintf(&b, "\t%q: {\n", slug)
		n := 0
		for _, id := range ids {
			m := prov.Models[id]
			if !m.ToolCall || m.Limit.Context < modelsdev.MinContextTokens {
				continue
			}
			name := m.Name
			if name == "" {
				name = id
			}
			efforts := effortValues(m)
			effortsLit := "nil"
			if len(efforts) > 0 {
				quoted := make([]string, len(efforts))
				for i, v := range efforts {
					quoted[i] = fmt.Sprintf("%q", v)
				}
				effortsLit = "[]string{" + strings.Join(quoted, ", ") + "}"
			}
			fmt.Fprintf(&b, "\t\t{ID: %q, Name: %q, ContextLimit: %d, OutputLimit: %d, InputPrice: %.4f, OutputPrice: %.4f, CacheReadPrice: %.4f, CacheWritePrice: %.4f, Reasoning: %t, Efforts: %s, Attachment: %t, Knowledge: %q, Released: %q},\n",
				id, name, m.Limit.Context, m.Limit.Output, m.Cost.Input, m.Cost.Output, m.Cost.CacheRead, m.Cost.CacheWrite, m.Reasoning, effortsLit, m.Attachment, m.Knowledge, m.ReleaseDate)
			n++
		}
		fmt.Fprintf(&b, "\t},\n")
		fmt.Fprintf(os.Stderr, "%s: %d models\n", slug, n)
		total += n
	}
	fmt.Fprintf(&b, "}\n")

	if err := os.WriteFile(*out, b.Bytes(), 0o644); err != nil {
		fatal("write %s: %v", *out, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d models)\n", *out, total)
}

func fetch() []byte {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(sourceURL)
	if err != nil {
		fatal("fetch catalog: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fatal("fetch catalog: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fatal("read catalog: %v", err)
	}
	if !json.Valid(data) {
		fatal("catalog is not valid JSON")
	}
	return data
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "modelsdev-gen: "+strings.TrimSpace(format)+"\n", args...)
	os.Exit(1)
}
