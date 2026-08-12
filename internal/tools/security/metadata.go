package security

func (t *SecretsScanTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *SecretsScanTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *SecretsScanTool) IsDestructive(args map[string]any) bool     { return false }

func (t *DependencyAuditTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *DependencyAuditTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *DependencyAuditTool) IsDestructive(args map[string]any) bool     { return false }
