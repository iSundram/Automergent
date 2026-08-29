package filesystem

func (t *GlobTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *GlobTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *GlobTool) IsDestructive(args map[string]any) bool     { return false }

func (t *GrepTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *GrepTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *GrepTool) IsDestructive(args map[string]any) bool     { return false }
