package prompt

import (
	"fmt"
	"strings"
)

// ToolPrompts generates prompts for specific tool operations.
type ToolPrompts struct {
	config *PromptConfig
}

// NewToolPrompts creates a new tool prompt generator.
func NewToolPrompts(config *PromptConfig) *ToolPrompts {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &ToolPrompts{config: config}
}

// BuildReadFilePrompt creates a prompt for reading a file.
func (t *ToolPrompts) BuildReadFilePrompt(path string, startLine, endLine int) *PromptPart {
	var sb strings.Builder

	sb.WriteString("READ FILE\n\n")
	sb.WriteString("Path: ")
	sb.WriteString(path)
	sb.WriteString("\n")

	if startLine > 0 || endLine > 0 {
		sb.WriteString("Lines: ")
		if startLine > 0 {
			sb.WriteString(fmt.Sprintf("%d", startLine))
		}
		sb.WriteString("-")
		if endLine > 0 {
			sb.WriteString(fmt.Sprintf("%d", endLine))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nRead the file and return its content.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetReadOnly,
		Metadata: map[string]any{
			"tool":       "read_file",
			"path":       path,
			"start_line": startLine,
			"end_line":   endLine,
		},
	}
}

// BuildReadManyFilesPrompt creates a prompt for reading multiple files.
func (t *ToolPrompts) BuildReadManyFilesPrompt(paths []string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("READ MULTIPLE FILES\n\n")
	sb.WriteString("Paths:\n")
	for _, p := range paths {
		sb.WriteString("  - ")
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	sb.WriteString("\nRead all files and return their contents.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetReadOnly,
		Metadata: map[string]any{
			"tool":  "read_many_files",
			"paths": paths,
		},
	}
}

// BuildReadFileLinesPrompt creates a prompt for reading specific lines from a file.
func (t *ToolPrompts) BuildReadFileLinesPrompt(path string, lineRanges [][2]int) *PromptPart {
	var sb strings.Builder

	sb.WriteString("READ FILE LINES\n\n")
	sb.WriteString("Path: ")
	sb.WriteString(path)
	sb.WriteString("\n")
	sb.WriteString("Line Ranges:\n")
	for _, r := range lineRanges {
		sb.WriteString(fmt.Sprintf("  %d-%d\n", r[0], r[1]))
	}
	sb.WriteString("\nRead the specified line ranges from the file.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetReadOnly,
		Metadata: map[string]any{
			"tool":        "read_file_lines",
			"path":        path,
			"line_ranges": lineRanges,
		},
	}
}

// BuildWriteFilePrompt creates a prompt for writing a file.
func (t *ToolPrompts) BuildWriteFilePrompt(path string, content string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("WRITE FILE\n\n")
	sb.WriteString("Path: ")
	sb.WriteString(path)
	sb.WriteString("\n\n")
	sb.WriteString("Content:\n")
	sb.WriteString(content)
	sb.WriteString("\n\nWrite the content to the file.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetBasic,
		Metadata: map[string]any{
			"tool":    "write_file",
			"path":    path,
			"content": content,
		},
	}
}

// BuildEditFilePrompt creates a prompt for editing a file.
func (t *ToolPrompts) BuildEditFilePrompt(path string, oldStr, newStr string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("EDIT FILE\n\n")
	sb.WriteString("Path: ")
	sb.WriteString(path)
	sb.WriteString("\n\n")
	sb.WriteString("Old String:\n")
	sb.WriteString(oldStr)
	sb.WriteString("\n\n")
	sb.WriteString("New String:\n")
	sb.WriteString(newStr)
	sb.WriteString("\n\nReplace the old string with the new string in the file.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetBasic,
		Metadata: map[string]any{
			"tool":     "edit_file",
			"path":     path,
			"old_str":  oldStr,
			"new_str":  newStr,
		},
	}
}

// BuildBashPrompt creates a prompt for executing a bash command.
func (t *ToolPrompts) BuildBashPrompt(command string, description string, timeout int) *PromptPart {
	var sb strings.Builder

	sb.WriteString("EXECUTE BASH COMMAND\n\n")
	sb.WriteString("Command: ")
	sb.WriteString(command)
	sb.WriteString("\n")
	if description != "" {
		sb.WriteString("Description: ")
		sb.WriteString(description)
		sb.WriteString("\n")
	}
	if timeout > 0 {
		sb.WriteString(fmt.Sprintf("Timeout: %d seconds\n", timeout))
	}
	sb.WriteString("\nExecute the command and return the output.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetModerate,
		Metadata: map[string]any{
			"tool":        "bash",
			"command":     command,
			"description": description,
			"timeout":     timeout,
		},
	}
}

// BuildSearchPrompt creates a prompt for searching code.
func (t *ToolPrompts) BuildSearchPrompt(pattern string, path string, includePattern string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("SEARCH CODE\n\n")
	sb.WriteString("Pattern: ")
	sb.WriteString(pattern)
	sb.WriteString("\n")
	if path != "" {
		sb.WriteString("Path: ")
		sb.WriteString(path)
		sb.WriteString("\n")
	}
	if includePattern != "" {
		sb.WriteString("Include Pattern: ")
		sb.WriteString(includePattern)
		sb.WriteString("\n")
	}
	sb.WriteString("\nSearch for the pattern and return matching locations.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetModerate,
		Metadata: map[string]any{
			"tool":            "search",
			"pattern":         pattern,
			"path":            path,
			"include_pattern": includePattern,
		},
	}
}

// BuildSQLPrompt creates a prompt for executing SQL.
func (t *ToolPrompts) BuildSQLPrompt(query string, database string) *PromptPart {
	var sb strings.Builder

	sb.WriteString("EXECUTE SQL\n\n")
	sb.WriteString("Query: ")
	sb.WriteString(query)
	sb.WriteString("\n")
	if database != "" {
		sb.WriteString("Database: ")
		sb.WriteString(database)
		sb.WriteString("\n")
	}
	sb.WriteString("\nExecute the SQL query and return results.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetModerate,
		Metadata: map[string]any{
			"tool":     "sql",
			"query":    query,
			"database": database,
		},
	}
}

// BuildToolSequencePrompt creates a prompt for a sequence of tool operations.
func (t *ToolPrompts) BuildToolSequencePrompt(operations []ToolOperation) *PromptPart {
	var sb strings.Builder

	sb.WriteString("TOOL SEQUENCE\n\n")
	sb.WriteString(fmt.Sprintf("Operations: %d\n\n", len(operations)))

	for i, op := range operations {
		sb.WriteString(fmt.Sprintf("Step %d: %s\n", i+1, op.Tool))
		sb.WriteString(fmt.Sprintf("  Description: %s\n", op.Description))
		for k, v := range op.Parameters {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Execute these operations in sequence. Report results after each step.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   t.determineToolSet(operations),
		Metadata: map[string]any{
			"tool":       "sequence",
			"operations": operations,
		},
	}
}

// ToolOperation represents a single tool operation in a sequence.
type ToolOperation struct {
	Tool        string
	Description string
	Parameters  map[string]any
}

func (t *ToolPrompts) determineToolSet(operations []ToolOperation) ToolSet {
	maxLevel := ToolSetContextOnly
	levels := map[string]ToolSet{
		"read_file":         ToolSetReadOnly,
		"read_many_files":   ToolSetReadOnly,
		"read_file_lines":   ToolSetReadOnly,
		"search":            ToolSetModerate,
		"write_file":        ToolSetBasic,
		"edit_file":         ToolSetBasic,
		"bash":              ToolSetModerate,
		"sql":               ToolSetModerate,
	}

	for _, op := range operations {
		if level, ok := levels[op.Tool]; ok {
			if level > maxLevel {
				maxLevel = level
			}
		}
	}
	return maxLevel
}

// BuildFileOperationPrompt creates a prompt for file operations (create, delete, move).
func (t *ToolPrompts) BuildFileOperationPrompt(operation, path, destination string) *PromptPart {
	var sb strings.Builder

	sb.WriteString(strings.ToUpper(operation))
	sb.WriteString(" FILE\n\n")
	sb.WriteString("Path: ")
	sb.WriteString(path)
	sb.WriteString("\n")
	if destination != "" {
		sb.WriteString("Destination: ")
		sb.WriteString(destination)
		sb.WriteString("\n")
	}
	sb.WriteString("\nPerform the file operation.")

	return &PromptPart{
		Stage:   StageExecution,
		Content: sb.String(),
		Tools:   ToolSetBasic,
		Metadata: map[string]any{
			"tool":         "file_operation",
			"operation":    operation,
			"path":         path,
			"destination":  destination,
		},
	}
}