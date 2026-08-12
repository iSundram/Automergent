package web

func (t *FetchTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *FetchTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *FetchTool) IsDestructive(args map[string]any) bool     { return false }

func (t *SearchTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *SearchTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *SearchTool) IsDestructive(args map[string]any) bool     { return false }
