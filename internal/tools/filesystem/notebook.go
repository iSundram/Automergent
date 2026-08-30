package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
)

// NotebookEditTool edits Jupyter notebook (.ipynb) cells in place: replace a
// cell's source, insert a new cell, or delete one. Cells are addressed by
// cell id when present, falling back to zero-based index.
type NotebookEditTool struct {
	cfg *config.Config
}

func NewNotebookEditTool(cfg *config.Config) *NotebookEditTool {
	return &NotebookEditTool{cfg: cfg}
}

func (t *NotebookEditTool) Name() string { return "notebook_edit" }
func (t *NotebookEditTool) Description() string {
	return `Edit a Jupyter notebook (.ipynb) cell: replace its source, insert a new cell, or delete it.
- Address cells by cell id when the notebook has them; otherwise by zero-based cell_index.
- edit_mode: "replace" (default) overwrites the cell's source; "insert" adds a new cell (new_source required, cell_type defaults to "code"); "delete" removes the cell.
- new_source is the full new cell source.`
}
func (t *NotebookEditTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}
func (t *NotebookEditTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *NotebookEditTool) IsReadOnly(args map[string]any) bool         { return false }
func (t *NotebookEditTool) IsDestructive(args map[string]any) bool {
	mode, _ := tools.StringArg(args, "edit_mode")
	return mode == "delete"
}

func (t *NotebookEditTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 300, LatencyMs: 50, RiskLevel: "medium"}
}

func (t *NotebookEditTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "edit",
		Usage:      "Prefer cell-level edits over rewriting the whole .ipynb with write_file — rewriting risks dropping outputs and metadata.",
		WhenToUse:  "The user asks to change, add or remove notebook cells.",
		WhenNotTo:  "For non-notebook files use edit_file / write_file.",
		InjectOrder: 35,
	}
}

func (t *NotebookEditTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the .ipynb notebook file.",
			},
			"cell_id": map[string]any{
				"type":        "string",
				"description": "Cell id (when the notebook defines cell ids).",
			},
			"cell_index": map[string]any{
				"type":        "integer",
				"description": "Zero-based cell index (used when cell_id is absent).",
			},
			"new_source": map[string]any{
				"type":        "string",
				"description": "Full new source for the cell (replace/insert modes).",
			},
			"cell_type": map[string]any{
				"type":        "string",
				"description": "Cell type for insert mode: code | markdown (default code).",
			},
			"edit_mode": map[string]any{
				"type":        "string",
				"description": "replace | insert | delete (default replace).",
			},
		},
		"required": []string{"path"},
	}
}

// notebookDoc is the minimal .ipynb shape the tool manipulates. Unknown
// top-level keys are preserved by marshaling through map[string]any.
type notebookDoc struct {
	Cells []map[string]any `json:"cells"`
}

func (t *NotebookEditTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}
	if strings.ToLower(filepath.Ext(path)) != ".ipynb" {
		return tools.Result{IsError: true, Content: "notebook_edit only handles .ipynb files"}, nil
	}
	editMode, _ := tools.StringArg(args, "edit_mode")
	if editMode == "" {
		editMode = "replace"
	}
	cellID, _ := tools.StringArg(args, "cell_id")
	cellIndex, hasIndex := tools.ArgInt(args, "cell_index")
	newSource, hasSource := tools.StringArg(args, "new_source")
	cellType, _ := tools.StringArg(args, "cell_type")

	if t.cfg != nil {
		if err := validateWritePath(path, t.cfg.Security.BlockedWritePaths, t.cfg.Security.AllowedWritePaths); err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: %v", err)}, nil
	}
	var doc notebookDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("invalid notebook JSON: %v", err)}, nil
	}
	if doc.Cells == nil {
		doc.Cells = []map[string]any{}
	}

	idx := -1
	if cellID != "" {
		for i, c := range doc.Cells {
			if id, _ := c["id"].(string); id == cellID {
				idx = i
				break
			}
		}
		if idx < 0 && editMode != "insert" {
			return tools.Result{IsError: true, Content: fmt.Sprintf("cell id %q not found", cellID)}, nil
		}
	} else if hasIndex {
		idx = cellIndex
	} else if editMode != "insert" {
		return tools.Result{IsError: true, Content: "cell_id or cell_index is required"}, nil
	}
	if editMode != "insert" && (idx < 0 || idx >= len(doc.Cells)) {
		return tools.Result{IsError: true, Content: fmt.Sprintf("cell index %d out of range (0-%d)", idx, len(doc.Cells)-1)}, nil
	}

	switch editMode {
	case "replace":
		if !hasSource {
			return tools.Result{IsError: true, Content: "new_source is required for replace"}, nil
		}
		doc.Cells[idx]["source"] = splitSource(newSource)
		doc.Cells[idx]["outputs"] = []any{}
		doc.Cells[idx]["execution_count"] = nil
	case "insert":
		if !hasSource {
			return tools.Result{IsError: true, Content: "new_source is required for insert"}, nil
		}
		if cellType != "markdown" {
			cellType = "code"
		}
		cell := map[string]any{
			"cell_type": cellType,
			"metadata":  map[string]any{},
			"source":    splitSource(newSource),
		}
		if cellType == "code" {
			cell["outputs"] = []any{}
			cell["execution_count"] = nil
		}
		at := idx
		if at < 0 || at > len(doc.Cells) {
			at = len(doc.Cells) // append when no addressable position
		}
		doc.Cells = append(doc.Cells[:at], append([]map[string]any{cell}, doc.Cells[at:]...)...)
		idx = at
	case "delete":
		doc.Cells = append(doc.Cells[:idx], doc.Cells[idx+1:]...)
	default:
		return tools.Result{IsError: true, Content: fmt.Sprintf("unknown edit_mode %q (replace|insert|delete)", editMode)}, nil
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("marshal: %v", err)}, nil
	}
	// Preserve the trailing newline notebooks conventionally carry.
	out = append(out, '\n')
	if err := atomicWriteFile(path, out, 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: %v", err)}, nil
	}

	return tools.Result{
		Content:  fmt.Sprintf("%s: %s cell %d (%d cells total)", path, editMode, idx, len(doc.Cells)),
		Summary:  fmt.Sprintf("%s cell %d", editMode, idx),
		Metadata: map[string]any{"cells": len(doc.Cells)},
	}, nil
}

// splitSource stores source in the notebook's canonical line-array form.
func splitSource(s string) []string {
	if s == "" {
		return []string{""}
	}
	lines := strings.Split(s, "\n")
	for i := 0; i < len(lines)-1; i++ {
		lines[i] += "\n"
	}
	return lines
}
