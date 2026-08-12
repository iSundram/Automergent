package lsp

func (t *DiagnosticsTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *DiagnosticsTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *DiagnosticsTool) IsDestructive(args map[string]any) bool     { return false }
