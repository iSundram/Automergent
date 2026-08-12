package database

import "strings"

func queryKind(query string) string {
	query = strings.TrimSpace(strings.ToUpper(query))
	switch {
	case strings.HasPrefix(query, "SELECT"):
		return "select"
	case strings.HasPrefix(query, "INSERT"):
		return "insert"
	case strings.HasPrefix(query, "UPDATE"):
		return "update"
	case strings.HasPrefix(query, "DELETE"):
		return "delete"
	case strings.HasPrefix(query, "CREATE"):
		return "create"
	case strings.HasPrefix(query, "DROP"):
		return "drop"
	case strings.HasPrefix(query, "ALTER"):
		return "alter"
	case strings.HasPrefix(query, "TRUNCATE"):
		return "truncate"
	default:
		return "other"
	}
}

func (t *SQLTool) IsConcurrencySafe(args map[string]any) bool {
	query, _ := args["query"].(string)
	return queryKind(query) == "select"
}

func (t *SQLTool) IsReadOnly(args map[string]any) bool {
	query, _ := args["query"].(string)
	return queryKind(query) == "select"
}

func (t *SQLTool) IsDestructive(args map[string]any) bool {
	query, _ := args["query"].(string)
	return queryKind(query) != "select"
}
