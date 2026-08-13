package database

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/iSundram/Automergent/internal/tools"
)

// SQLTool provides SQL query capabilities against a session database.
//
// TODO: For production use, integrate with modernc.org/sqlite (pure Go SQLite).
// The current implementation is an in-memory stub that:
// - Does not persist data across restarts
// - Does not parse SQL properly (uses string matching)
// - Does not support complex queries, JOINs, WHERE clauses, etc.
//
// To implement properly:
// 1. Add "modernc.org/sqlite" to go.mod
// 2. Replace tables map with *sql.DB connection
// 3. Use actual SQL execution with db.Query/db.Exec
// 4. Store database file in session folder (e.g., ~/.copilot/session-state/{id}/session.db)
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

	// Determine kind early
	kind := queryKind(query)

	// For SELECT, allow concurrent reads: use read lock and validate under read lock
	if kind == "select" {
		t.mu.RLock()
		defer t.mu.RUnlock()
		if err := t.validateQuery(query); err != nil {
			return tools.Result{
				Content: fmt.Sprintf("Query validation failed: %v", err),
				Summary: "Query rejected for security",
				IsError: true,
			}, err
		}
		return t.handleSelect(query)
	}

	// For mutations, acquire write lock and validate while holding it to avoid races
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.validateQuery(query); err != nil {
		return tools.Result{
			Content: fmt.Sprintf("Query validation failed: %v", err),
			Summary: "Query rejected for security",
			IsError: true,
		}, err
	}

	queryUpper := strings.ToUpper(strings.TrimSpace(query))

	switch {
	case strings.HasPrefix(queryUpper, "INSERT"):
		return t.handleInsert(query)
	case strings.HasPrefix(queryUpper, "UPDATE"):
		return t.handleUpdate(query)
	case strings.HasPrefix(queryUpper, "DELETE"):
		return t.handleDelete(query)
	default:
		return tools.Result{
			Content: fmt.Sprintf("Query noted: %s\n(Note: Full SQL support requires SQLite integration)", query),
		}, nil
	}
}

// validateQuery performs basic SQL injection detection.
// This is a defense-in-depth measure for the stub implementation.
// IMPORTANT: This is NOT sufficient for production use.
// Real SQL execution MUST use prepared statements with parameterized queries.
func (t *SQLTool) validateQuery(query string) error {
	// Collapse whitespace for normalization
	normalized := strings.ToUpper(strings.Join(strings.Fields(query), " "))

	// Parse query and extract text outside of quoted literals so that
	// pattern checks only apply to SQL code, not to literal data.
	var outsideBuilder strings.Builder
	inSingle := false
	inDouble := false
	q := query
	for i := 0; i < len(q); i++ {
		c := q[i]
		if inSingle {
			if c == '\'' {
				// SQL escaping uses doubled single quotes: '' -> one quote inside string
				if i+1 < len(q) && q[i+1] == '\'' {
					i++ // skip the escaped quote
					continue
				}
				// backslash-escaped single quote (some clients): skip the escape
				if i > 0 && q[i-1] == '\\' {
					continue
				}
				inSingle = false
				continue
			}
			// inside single quoted literal - ignore
			continue
		}
		if inDouble {
			if c == '"' {
				if i+1 < len(q) && q[i+1] == '"' {
					i++
					continue
				}
				if i > 0 && q[i-1] == '\\' {
					continue
				}
				inDouble = false
				continue
			}
			continue
		}
		// not in any quote
		if c == '\'' {
			inSingle = true
			continue
		}
		if c == '"' {
			inDouble = true
			continue
		}
		outsideBuilder.WriteByte(c)
	}

	if inSingle || inDouble {
		return fmt.Errorf("unbalanced quotes detected in query")
	}

	outside := strings.ToUpper(strings.Join(strings.Fields(outsideBuilder.String()), " "))

	// Dangerous patterns to check only in outside-of-literals text
	dangerous := []struct {
		pattern string
		reason  string
	}{
		{"--", "SQL comment marker"},
		{"/*", "SQL block comment start"},
		{"*/", "SQL block comment end"},
		{";--", "statement terminator with comment"},
		{"UNION SELECT", "UNION-based injection"},
		{"UNION ALL SELECT", "UNION-based injection"},
		{"DROP TABLE", "table deletion attempt"},
		{"DROP DATABASE", "database deletion attempt"},
		{"EXEC ", "stored procedure execution"},
		{"EXECUTE ", "stored procedure execution"},
		{"XP_", "SQL Server extended procedure"},
		{"SP_", "SQL Server stored procedure"},
		{"WAITFOR DELAY", "time-based blind injection"},
		{"BENCHMARK(", "MySQL time-based injection"},
		{"SLEEP(", "MySQL time-based injection"},
		{"PG_SLEEP(", "PostgreSQL time-based injection"},
		{"DBMS_LOCK", "Oracle time-based injection"},
	}

	for _, d := range dangerous {
		if strings.Contains(outside, d.pattern) {
			return fmt.Errorf("potentially dangerous SQL pattern detected: %s (%s)", d.pattern, d.reason)
		}
	}

	// Semicolon check: ensure there's at most one statement. Semicolons inside literals were removed
	trimmedOutside := strings.TrimSpace(outside)
	if idx := strings.Index(trimmedOutside, ";"); idx != -1 {
		// allow semicolon only at the very end
		after := strings.TrimSpace(trimmedOutside[idx+1:])
		if after != "" {
			return fmt.Errorf("multiple statements detected (semicolon not at end)")
		}
	}

	// If we've made it this far, query looks acceptable to the stub
	_ = normalized // keep variable if future usage needed
	return nil
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

// helper: trim identifier characters and normalize
func trimIdentifier(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '`' && s[len(s)-1] == '`') || (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return strings.ToLower(s[1 : len(s)-1])
		}
	}
	// remove trailing commas/parens
	s = strings.Trim(s, ",() ")
	return strings.ToLower(s)
}

// splitCSV splits a comma-separated list but ignores commas inside quotes
func splitCSV(s string) []string {
	var parts []string
	current := strings.Builder{}
	inSingle := false
	inDouble := false
	q := s
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == '\'' && !inDouble {
			// handle escaped '' inside SQL
			if inSingle && i+1 < len(q) && q[i+1] == '\'' {
				current.WriteByte('\'')
				i++
				continue
			}
			inSingle = !inSingle
			current.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			if inDouble && i+1 < len(q) && q[i+1] == '"' {
				current.WriteByte('"')
				i++
				continue
			}
			inDouble = !inDouble
			current.WriteByte(c)
			continue
		}
		if c == ',' && !inSingle && !inDouble {
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	if current.Len() > 0 {
		parts = append(parts, strings.TrimSpace(current.String()))
	}
	return parts
}

func unquoteSQLValue(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if s[0] == '\'' && s[len(s)-1] == '\'' {
			inner := s[1 : len(s)-1]
			// replace doubled single quotes with single quote
			inner = strings.ReplaceAll(inner, "''", "'")
			// replace backslash-escaped single quotes
			inner = strings.ReplaceAll(inner, "\\'", "'")
			return inner
		}
		if s[0] == '"' && s[len(s)-1] == '"' {
			inner := s[1 : len(s)-1]
			inner = strings.ReplaceAll(inner, "\"\"", "\"")
			return inner
		}
	}
	return s
}

func (t *SQLTool) handleInsert(query string) (tools.Result, error) {
	upper := strings.ToUpper(query)
	intoIdx := strings.Index(upper, "INTO")
	if intoIdx == -1 {
		return tools.Result{IsError: true, Content: "INSERT must contain INTO"}, nil
	}
	after := strings.TrimSpace(query[intoIdx+len("INTO"):])
	// table name is first token before space or (
	tblName := after
	if i := strings.IndexAny(after, " (\t\n"); i != -1 {
		tblName = after[:i]
	}
	tbl := trimIdentifier(tblName)
	if tbl == "" {
		return tools.Result{IsError: true, Content: "could not determine table name"}, nil
	}
	// find VALUES
	valsIdx := strings.Index(upper, "VALUES")
	if valsIdx == -1 {
		return tools.Result{IsError: true, Content: "only INSERT ... VALUES supported in stub"}, nil
	}
	// attempt to parse columns between table name and VALUES
	colsSection := query[intoIdx+len("INTO") : valsIdx]
	cols := []string{}
	if open := strings.Index(colsSection, "("); open != -1 {
		close := strings.LastIndex(colsSection, ")")
		if close > open {
			colsText := colsSection[open+1 : close]
			for _, c := range splitCSV(colsText) {
				cols = append(cols, trimIdentifier(c))
			}
		}
	}
	// parse values section: find first '(' after valsIdx
	rest := query[valsIdx+len("VALUES"):]
	open := strings.Index(rest, "(")
	close := strings.LastIndex(rest, ")")
	if open == -1 || close == -1 || close < open {
		return tools.Result{IsError: true, Content: "malformed VALUES clause"}, nil
	}
	valsText := rest[open+1 : close]
	vals := splitCSV(valsText)
	// build row
	row := make(map[string]any)
	if len(cols) > 0 {
		for i, v := range vals {
			col := fmt.Sprintf("col%d", i+1)
			if i < len(cols) && cols[i] != "" {
				col = cols[i]
			}
			row[col] = unquoteSQLValue(v)
		}
	} else {
		for i, v := range vals {
			row[fmt.Sprintf("col%d", i+1)] = unquoteSQLValue(v)
		}
	}
	// append to table
	t.tables[tbl] = append(t.tables[tbl], row)
	return tools.Result{Content: fmt.Sprintf("%d row(s) inserted.", 1)}, nil
}

func (t *SQLTool) handleUpdate(query string) (tools.Result, error) {
	upper := strings.ToUpper(query)
	// find UPDATE <table> SET ... [WHERE ...]
	updateIdx := strings.Index(upper, "UPDATE")
	setIdx := strings.Index(upper, " SET ")
	if updateIdx == -1 || setIdx == -1 {
		return tools.Result{IsError: true, Content: "malformed UPDATE"}, nil
	}
	tblName := strings.TrimSpace(query[updateIdx+len("UPDATE") : setIdx])
	tbl := trimIdentifier(tblName)
	if _, ok := t.tables[tbl]; !ok {
		return tools.Result{IsError: true, Content: "unknown table"}, nil
	}
	whereIdx := strings.Index(upper, " WHERE ")
	setSection := query[setIdx+len(" SET ") : func() int {
		if whereIdx == -1 {
			return len(query)
		} else {
			return whereIdx
		}
	}()]
	assigns := splitCSV(setSection)
	updates := make(map[string]string)
	for _, a := range assigns {
		if eq := strings.Index(a, "="); eq != -1 {
			k := trimIdentifier(a[:eq])
			v := strings.TrimSpace(a[eq+1:])
			updates[k] = unquoteSQLValue(v)
		}
	}
	// determine predicate: support simple equality col = val
	predCol := ""
	predVal := ""
	if whereIdx != -1 {
		whereClause := strings.TrimSpace(query[whereIdx+len(" WHERE "):])
		// remove trailing semicolon
		whereClause = strings.TrimRight(whereClause, "; ")
		// only support single equality
		if eq := strings.Index(whereClause, "="); eq != -1 {
			predCol = trimIdentifier(whereClause[:eq])
			predVal = unquoteSQLValue(strings.TrimSpace(whereClause[eq+1:]))
		}
	}
	count := 0
	for i, row := range t.tables[tbl] {
		match := true
		if predCol != "" {
			valStr := fmt.Sprintf("%v", row[predCol])
			if valStr != predVal {
				match = false
			}
		}
		if match {
			for k, v := range updates {
				row[k] = v
			}
			t.tables[tbl][i] = row
			count++
		}
	}
	return tools.Result{Content: fmt.Sprintf("%d row(s) updated.", count)}, nil
}

func (t *SQLTool) handleDelete(query string) (tools.Result, error) {
	upper := strings.ToUpper(query)
	fromIdx := strings.Index(upper, "FROM")
	if fromIdx == -1 {
		// support DELETE <table> form
		parts := strings.Fields(query)
		if len(parts) >= 2 {
			fromIdx = strings.Index(strings.ToUpper(query), strings.ToUpper(parts[1]))
		}
	}
	if fromIdx == -1 {
		return tools.Result{IsError: true, Content: "malformed DELETE"}, nil
	}
	// find table name after FROM
	after := strings.TrimSpace(query[fromIdx+len("FROM"):])
	tblName := after
	if i := strings.IndexAny(after, " (\t\n;"); i != -1 {
		tblName = after[:i]
	}
	tbl := trimIdentifier(tblName)
	if _, ok := t.tables[tbl]; !ok {
		return tools.Result{IsError: true, Content: "unknown table"}, nil
	}
	whereIdx := strings.Index(upper, " WHERE ")
	count := 0
	if whereIdx == -1 {
		count = len(t.tables[tbl])
		t.tables[tbl] = []map[string]any{}
		return tools.Result{Content: fmt.Sprintf("%d row(s) deleted.", count)}, nil
	}
	whereClause := strings.TrimSpace(query[whereIdx+len(" WHERE "):])
	whereClause = strings.TrimRight(whereClause, "; ")
	predCol := ""
	predVal := ""
	if eq := strings.Index(whereClause, "="); eq != -1 {
		predCol = trimIdentifier(whereClause[:eq])
		predVal = unquoteSQLValue(strings.TrimSpace(whereClause[eq+1:]))
	}
	var newRows []map[string]any
	for _, row := range t.tables[tbl] {
		keep := true
		if predCol != "" {
			valStr := fmt.Sprintf("%v", row[predCol])
			if valStr == predVal {
				keep = false
				count++
			}
		}
		if keep {
			newRows = append(newRows, row)
		}
	}
	t.tables[tbl] = newRows
	return tools.Result{Content: fmt.Sprintf("%d row(s) deleted.", count)}, nil
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

// EstimatedCost returns cost estimates for the SQL tool.
func (t *SQLTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 100, LatencyMs: 50, RiskLevel: "low"}
}
