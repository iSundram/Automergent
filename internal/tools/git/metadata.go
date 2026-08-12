package git

import "github.com/iSundram/Automergent/internal/tools"

func (t *CommitTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *CommitTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *CommitTool) IsDestructive(args map[string]any) bool     { return true }

func (t *AddTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *AddTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *AddTool) IsDestructive(args map[string]any) bool     { return false }

func (t *CheckoutTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *CheckoutTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *CheckoutTool) IsDestructive(args map[string]any) bool     { return true }

func (t *BranchTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *BranchTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *BranchTool) IsDestructive(args map[string]any) bool {
	action, _ := tools.StringArg(args, "action")
	return action == "delete"
}

func (t *StashTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *StashTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *StashTool) IsDestructive(args map[string]any) bool {
	action, _ := tools.StringArg(args, "action")
	return action == "pop" || action == "apply" || action == "drop"
}

func (t *BlameTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *BlameTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *BlameTool) IsDestructive(args map[string]any) bool     { return false }

func (t *ShowTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *ShowTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ShowTool) IsDestructive(args map[string]any) bool     { return false }
