// Package docs provides auto-documentation generation for tools.
package docs

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/pkg/sdk/builder"
	"github.com/iSundram/Automergent/pkg/sdk/schema"
)

// Generator generates documentation for tools.
type Generator struct {
	tools   []tools.Tool
	options *Options
}

// Options configures documentation generation.
type Options struct {
	Title           string
	Description     string
	Version         string
	IncludeExamples bool
	IncludeSchema   bool
	IncludeMetadata bool
	GroupByTags     bool
}

// DefaultOptions returns default documentation options.
func DefaultOptions() *Options {
	return &Options{
		Title:           "Tool Documentation",
		IncludeExamples: true,
		IncludeSchema:   true,
		IncludeMetadata: false,
		GroupByTags:     false,
	}
}

// NewGenerator creates a new documentation generator.
func NewGenerator(opts *Options) *Generator {
	if opts == nil {
		opts = DefaultOptions()
	}
	return &Generator{options: opts}
}

// AddTool adds a tool to document.
func (g *Generator) AddTool(t tools.Tool) *Generator {
	g.tools = append(g.tools, t)
	return g
}

// AddTools adds multiple tools to document.
func (g *Generator) AddTools(tools ...tools.Tool) *Generator {
	g.tools = append(g.tools, tools...)
	return g
}

// AddAllRegistered adds all tools from the global registry.
func (g *Generator) AddAllRegistered() *Generator {
	g.tools = append(g.tools, tools.All()...)
	return g
}

// GenerateMarkdown generates Markdown documentation.
func (g *Generator) GenerateMarkdown() string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# %s\n\n", g.options.Title))

	if g.options.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", g.options.Description))
	}

	if g.options.Version != "" {
		sb.WriteString(fmt.Sprintf("**Version:** %s\n\n", g.options.Version))
	}

	// Table of contents
	sb.WriteString("## Table of Contents\n\n")
	for _, t := range g.tools {
		sb.WriteString(fmt.Sprintf("- [%s](#%s)\n", t.Name(), strings.ReplaceAll(t.Name(), "_", "-")))
	}
	sb.WriteString("\n---\n\n")

	// Document each tool
	for _, t := range g.tools {
		sb.WriteString(g.generateToolDoc(t))
		sb.WriteString("\n---\n\n")
	}

	return sb.String()
}

func (g *Generator) generateToolDoc(t tools.Tool) string {
	var sb strings.Builder

	// Tool name and description
	sb.WriteString(fmt.Sprintf("## %s\n\n", t.Name()))
	sb.WriteString(fmt.Sprintf("%s\n\n", t.Description()))

	// Check if it's a FluentTool for additional metadata
	if ft, ok := t.(*builder.FluentTool); ok {
		if ft.IsDeprecated() {
			sb.WriteString(fmt.Sprintf("> ⚠️ **Deprecated:** %s\n\n", ft.DeprecationMessage()))
		}
		if ft.Version() != "" {
			sb.WriteString(fmt.Sprintf("**Version:** %s\n\n", ft.Version()))
		}
		if len(ft.Tags()) > 0 {
			sb.WriteString(fmt.Sprintf("**Tags:** %s\n\n", strings.Join(ft.Tags(), ", ")))
		}
	}

	// Parameters
	schema := t.Schema()
	if props, ok := schema["properties"].(map[string]any); ok && len(props) > 0 {
		sb.WriteString("### Parameters\n\n")
		sb.WriteString("| Name | Type | Required | Description |\n")
		sb.WriteString("|------|------|----------|-------------|\n")

		required := make(map[string]bool)
		if reqList, ok := schema["required"].([]string); ok {
			for _, r := range reqList {
				required[r] = true
			}
		}

		// Sort parameters for consistent output
		var paramNames []string
		for name := range props {
			paramNames = append(paramNames, name)
		}
		sort.Strings(paramNames)

		for _, name := range paramNames {
			prop := props[name].(map[string]any)
			pType := prop["type"]
			desc := ""
			if d, ok := prop["description"].(string); ok {
				desc = d
			}
			reqStr := "No"
			if required[name] {
				reqStr = "Yes"
			}
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", name, pType, reqStr, desc))
		}
		sb.WriteString("\n")
	}

	// Schema details
	if g.options.IncludeSchema {
		sb.WriteString("### Schema\n\n")
		sb.WriteString("```json\n")
		schemaJSON, _ := json.MarshalIndent(schema, "", "  ")
		sb.WriteString(string(schemaJSON))
		sb.WriteString("\n```\n\n")
	}

	// Examples
	if g.options.IncludeExamples {
		if ft, ok := t.(*builder.FluentTool); ok && len(ft.Examples()) > 0 {
			sb.WriteString("### Examples\n\n")
			for _, ex := range ft.Examples() {
				sb.WriteString(fmt.Sprintf("#### %s\n\n", ex.Name))
				if ex.Description != "" {
					sb.WriteString(fmt.Sprintf("%s\n\n", ex.Description))
				}
				sb.WriteString("**Arguments:**\n```json\n")
				argsJSON, _ := json.MarshalIndent(ex.Args, "", "  ")
				sb.WriteString(string(argsJSON))
				sb.WriteString("\n```\n\n")
				if ex.Expected != "" {
					sb.WriteString(fmt.Sprintf("**Expected:** %s\n\n", ex.Expected))
				}
			}
		}
	}

	// Confirmation
	confirmPlan := t.RequiresConfirmation("plan")
	confirmAct := t.RequiresConfirmation("act")
	if confirmPlan || confirmAct {
		sb.WriteString("### Confirmation\n\n")
		if confirmPlan && confirmAct {
			sb.WriteString("This tool always requires user confirmation before execution.\n\n")
		} else if confirmPlan {
			sb.WriteString("This tool requires confirmation in `plan` mode.\n\n")
		} else {
			sb.WriteString("This tool requires confirmation in `act` mode.\n\n")
		}
	}

	return sb.String()
}

// GenerateJSON generates JSON documentation.
func (g *Generator) GenerateJSON() (string, error) {
	type ParamDoc struct {
		Name        string   `json:"name"`
		Type        string   `json:"type"`
		Description string   `json:"description,omitempty"`
		Required    bool     `json:"required"`
		Default     any      `json:"default,omitempty"`
		Enum        []string `json:"enum,omitempty"`
	}

	type ExampleDoc struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Args        map[string]any `json:"args"`
		Expected    string         `json:"expected,omitempty"`
	}

	type ToolDoc struct {
		Name                 string         `json:"name"`
		Description          string         `json:"description"`
		Version              string         `json:"version,omitempty"`
		Tags                 []string       `json:"tags,omitempty"`
		Deprecated           bool           `json:"deprecated,omitempty"`
		DeprecationMessage   string         `json:"deprecation_message,omitempty"`
		Parameters           []ParamDoc     `json:"parameters"`
		Examples             []ExampleDoc   `json:"examples,omitempty"`
		RequiresConfirmation bool           `json:"requires_confirmation"`
		Schema               map[string]any `json:"schema"`
	}

	var docs []ToolDoc

	for _, t := range g.tools {
		doc := ToolDoc{
			Name:                 t.Name(),
			Description:          t.Description(),
			Schema:               t.Schema(),
			RequiresConfirmation: t.RequiresConfirmation("plan") || t.RequiresConfirmation("act"),
		}

		// Extract parameters
		if props, ok := doc.Schema["properties"].(map[string]any); ok {
			required := make(map[string]bool)
			if reqList, ok := doc.Schema["required"].([]string); ok {
				for _, r := range reqList {
					required[r] = true
				}
			}

			for name, prop := range props {
				p := prop.(map[string]any)
				pd := ParamDoc{
					Name:     name,
					Type:     fmt.Sprintf("%v", p["type"]),
					Required: required[name],
				}
				if desc, ok := p["description"].(string); ok {
					pd.Description = desc
				}
				if def := p["default"]; def != nil {
					pd.Default = def
				}
				if enum, ok := p["enum"].([]string); ok {
					pd.Enum = enum
				}
				doc.Parameters = append(doc.Parameters, pd)
			}
		}

		// FluentTool-specific fields
		if ft, ok := t.(*builder.FluentTool); ok {
			doc.Version = ft.Version()
			doc.Tags = ft.Tags()
			doc.Deprecated = ft.IsDeprecated()
			doc.DeprecationMessage = ft.DeprecationMessage()

			for _, ex := range ft.Examples() {
				doc.Examples = append(doc.Examples, ExampleDoc{
					Name:        ex.Name,
					Description: ex.Description,
					Args:        ex.Args,
					Expected:    ex.Expected,
				})
			}
		}

		docs = append(docs, doc)
	}

	output := map[string]any{
		"title":       g.options.Title,
		"description": g.options.Description,
		"version":     g.options.Version,
		"tools":       docs,
	}

	result, err := json.MarshalIndent(output, "", "  ")
	return string(result), err
}

// ToolDocumentation provides documentation for a single tool.
type ToolDocumentation struct {
	tool tools.Tool
}

// ForTool creates documentation for a single tool.
func ForTool(t tools.Tool) *ToolDocumentation {
	return &ToolDocumentation{tool: t}
}

// Markdown returns the Markdown documentation.
func (td *ToolDocumentation) Markdown() string {
	gen := NewGenerator(DefaultOptions())
	gen.AddTool(td.tool)
	return gen.generateToolDoc(td.tool)
}

// Schema returns the JSON schema as formatted JSON.
func (td *ToolDocumentation) Schema() string {
	schemaJSON, _ := json.MarshalIndent(td.tool.Schema(), "", "  ")
	return string(schemaJSON)
}

// ParameterDocs generates a parameter documentation table.
func ParameterDocs(params map[string]*schema.ParamSchema) string {
	var sb strings.Builder
	sb.WriteString("| Name | Type | Required | Description | Default |\n")
	sb.WriteString("|------|------|----------|-------------|----------|\n")

	var names []string
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		p := params[name]
		reqStr := "No"
		if p.Required {
			reqStr = "Yes"
		}
		defStr := "-"
		if p.Default != nil {
			defStr = fmt.Sprintf("`%v`", p.Default)
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
			name, p.Type, reqStr, p.Description, defStr))
	}

	return sb.String()
}
