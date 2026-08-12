package testing

func (t *RunTestsTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *RunTestsTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *RunTestsTool) IsDestructive(args map[string]any) bool     { return false }

func (t *TestCoverageTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *TestCoverageTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *TestCoverageTool) IsDestructive(args map[string]any) bool     { return true }
