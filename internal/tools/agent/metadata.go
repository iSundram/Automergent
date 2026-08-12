package agent

func (t *TaskTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *TaskTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *TaskTool) IsDestructive(args map[string]any) bool     { return false }

func (t *ReadAgentTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *ReadAgentTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ReadAgentTool) IsDestructive(args map[string]any) bool     { return false }

func (t *ListAgentsTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *ListAgentsTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ListAgentsTool) IsDestructive(args map[string]any) bool     { return false }
