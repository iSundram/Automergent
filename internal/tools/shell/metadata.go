package shell

import (
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

func shellCommandReadOnly(command string) bool {
	cmdLower := strings.ToLower(command)
	readOnlyCommands := []string{"ls", "cat", "grep", "find", "echo", "pwd", "git log", "git show", "git diff"}
	for _, readCmd := range readOnlyCommands {
		if strings.HasPrefix(cmdLower, readCmd) {
			return true
		}
	}
	return false
}

func shellCommandDestructive(command string) bool {
	cmdLower := strings.ToLower(command)
	destructivePatterns := []string{"rm -rf", "rm -r", "delete", "drop table", "truncate", "format"}
	for _, pattern := range destructivePatterns {
		if strings.Contains(cmdLower, pattern) {
			return true
		}
	}
	return false
}

func shellInputDestructive(input string) bool {
	return shellCommandDestructive(strings.ReplaceAll(strings.ToLower(input), "\n", " "))
}

func (t *AsyncRunnerTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *AsyncRunnerTool) IsReadOnly(args map[string]any) bool {
	command, ok := tools.StringArg(args, "command")
	return ok && shellCommandReadOnly(command)
}
func (t *AsyncRunnerTool) IsDestructive(args map[string]any) bool {
	command, ok := tools.StringArg(args, "command")
	return ok && shellCommandDestructive(command)
}

func (t *ReadShellTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *ReadShellTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ReadShellTool) IsDestructive(args map[string]any) bool     { return false }

func (t *WriteShellTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *WriteShellTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *WriteShellTool) IsDestructive(args map[string]any) bool {
	input, ok := tools.StringArg(args, "input")
	return ok && shellInputDestructive(input)
}

func (t *StopShellTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *StopShellTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *StopShellTool) IsDestructive(args map[string]any) bool     { return true }

func (t *ListShellsTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *ListShellsTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ListShellsTool) IsDestructive(args map[string]any) bool     { return false }
