package filesystem

import (
	"os"

	"github.com/iSundram/Automergent/internal/tools"
)

func (t *CreateFileTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *CreateFileTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *CreateFileTool) IsDestructive(args map[string]any) bool     { return false }

func (t *DeleteFileTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *DeleteFileTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *DeleteFileTool) IsDestructive(args map[string]any) bool     { return true }

func (t *MoveFileTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *MoveFileTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *MoveFileTool) IsDestructive(args map[string]any) bool     { return true }

func (t *CopyFileTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *CopyFileTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *CopyFileTool) IsDestructive(args map[string]any) bool {
	overwrite, _ := tools.ArgBool(args, "overwrite")
	if overwrite {
		return true
	}
	dest, _ := tools.StringArg(args, "destination")
	if dest == "" {
		return false
	}
	_, err := os.Stat(dest)
	return err == nil
}

func (t *ViewFileTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *ViewFileTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ViewFileTool) IsDestructive(args map[string]any) bool     { return false }

func (t *GlobTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *GlobTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *GlobTool) IsDestructive(args map[string]any) bool     { return false }

func (t *GrepTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *GrepTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *GrepTool) IsDestructive(args map[string]any) bool     { return false }

func (t *RefinedSearchTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *RefinedSearchTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *RefinedSearchTool) IsDestructive(args map[string]any) bool     { return false }
