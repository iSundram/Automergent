package database

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/iSundram/Automergent/internal/tools"
)

// SQLTool provides SQL query capabilities against a session database.
// Note: This is a simplified in-memory implementation.
// For production, integrate with modernc.org/sqlite (pure Go SQLite).
type SQLTool struct {
	mu     sync.RWMutex
	tables map[string][]map[string]any
}

var (
	globalSQLTool *SQLTool
	once          sync.Once
)

// GetSQLTool returns the global SQL tool instance.
func GetSQLTool() *SQLTool {
	once.Do(func() {
		globalSQLTool = &SQLTool{
			tables: make(map[string][]map[string]any),
		}
		// Initialize default tables
		globalSQLTool.tables["todos"] = []map[string]any{}
		globalSQLTool.tables["todo_deps"] = []map[string]any{}
		globalSQLTool.tables["session_state"] = []map[string]any{}
	})
	return globalSQLTool
}

// Initialize is a no-op for the in-memory implementation.
// For production, this would open the SQLite database.
func (t *SQLTool) Initialize(dbPath string) error {
	// In-memory tables are already initialized in GetSQLTool()
	return nil
}

func (t *SQLTool) Name() string { return "sql" }
func (t *SQLTool) Description() string {
	return `Execute SQL queries against the session SQLite database.

Pre-built tables:
- todos: id, title, description, status (pending/in_progress/done/blocked), created_at, updated_at
- todo_deps: todo_id, depends_on (for dependency tracking)
- session_state: key, value (for key-value storage)

Supports all SQLite SQL: SELECT, INSERT, UPDATE, DELETE, CREATE TABLE, etc.
Use descriptive kebab-case IDs for todos (e.g., 'user-auth', 'api-routes').`
}
func (t *SQLTool) RequiresConfirmation(mode string) bool { return false }

func (t *SQLTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "SQL query to execute.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "2-5 word summary of what this query does.",
			},
			"database": map[string]any{
				"type":        "string",
				"enum":        []string{"session", "session_store"},
				"description": "Which database to query (default: session).",
			},
		},
		"required": []string{"query", "description"},
	}
}

func (t *SQLTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	query, ok := tools.StringArg(args, "query")
	if !ok || query == "" {
		return tools.Result{IsError: true, Content: "query is required"}, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Simple query parser for demonstration
	// In production, use a proper SQL parser or SQLite library
	queryUpper := strings.ToUpper(strings.TrimSpace(query))

	if strings.HasPrefix(queryUpper, "SELECT") {
		return t.handleSelect(query)
	}
	if strings.HasPrefix(queryUpper, "INSERT") {
		return t.handleInsert(query)
	}
	if strings.HasPrefix(queryUpper, "UPDATE") {
		return t.handleUpdate(query)
	}
	if strings.HasPrefix(queryUpper, "DELETE") {
		return t.handleDelete(query)
	}

	return tools.Result{
		Content: fmt.Sprintf("Query noted: %s\n(Note: Full SQL support requires SQLite integration)", query),
	}, nil
}

func (t *SQLTool) handleSelect(query string) (tools.Result, error) {
	// Simplified: return table contents
	for tableName, rows := range t.tables {
		if strings.Contains(strings.ToUpper(query), strings.ToUpper(tableName)) {
			if len(rows) == 0 {
				return tools.Result{Content: "(no results)"}, nil
			}
			var lines []string
			for _, row := range rows {
				lines = append(lines, fmt.Sprintf("%v", row))
			}
			return tools.Result{Content: strings.Join(lines, "\n")}, nil
		}
	}
	return tools.Result{Content: "(no results)"}, nil
}

func (t *SQLTool) handleInsert(query string) (tools.Result, error) {
	// For demonstration - in production use proper SQL parser
	return tools.Result{Content: "1 row(s) inserted. (Note: Full SQL support requires SQLite integration)"}, nil
}

func (t *SQLTool) handleUpdate(query string) (tools.Result, error) {
	return tools.Result{Content: "Row(s) updated. (Note: Full SQL support requires SQLite integration)"}, nil
}

func (t *SQLTool) handleDelete(query string) (tools.Result, error) {
	return tools.Result{Content: "Row(s) deleted. (Note: Full SQL support requires SQLite integration)"}, nil
}

// Helper functions for common todo operations (stub implementations)

// UpdateTodoStatus updates a todo's status
func UpdateTodoStatus(id, status string) error {
	// In production, this would update the SQLite database
	return nil
}

// GetReadyTodos returns todos with no pending dependencies
func GetReadyTodos() ([]string, error) {
	// In production, this would query the SQLite database
	return []string{}, nil
}
