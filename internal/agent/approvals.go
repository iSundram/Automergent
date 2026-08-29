package agent

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/tools"
)

// This file implements grant generalization for "always allow" approvals:
// approving a command like `go test` auto-approves future `go test ...`
// invocations (token-prefix match). Commands containing dangerous constructs
// are never generalized — their grants match only the exact same command.

// shellToolNames are the tools whose approval scope carries the executed
// command and participates in prefix generalization.
var shellToolNames = map[string]bool{
	"bash": true,
}

// dangerousCommandPatterns disable prefix generalization: a grant for these
// matches only the exact same command string.
var dangerousCommandSubstrings = []string{
	"$(", "`", "sudo ", "eval ", "xargs ",
	"rm -rf", "rm -fr", "mkfs", "dd if=", "> /dev/", "| sh", "| bash",
	"chmod 777", ":(){", "curl |", "wget |", ">> /etc/", "/etc/passwd", "/etc/shadow",
}

// shellCommandOf extracts the command string from a shell tool call.
func shellCommandOf(name string, args map[string]any) string {
	if !shellToolNames[name] {
		return ""
	}
	if cmd, ok := tools.StringArg(args, "command"); ok {
		return strings.TrimSpace(cmd)
	}
	return ""
}

// isGeneralizable reports whether a command may be approved by prefix.
func isGeneralizable(cmd string) bool {
	if cmd == "" {
		return false
	}
	lower := strings.ToLower(cmd)
	for _, pat := range dangerousCommandSubstrings {
		if strings.Contains(lower, pat) {
			return false
		}
	}
	return true
}

// commandPrefix returns the leading tokens used for prefix matching:
// binary + first subcommand ("git commit" from "git commit -m 'x'").
func commandPrefix(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) > 2 {
		fields = fields[:2]
	}
	return strings.Join(fields, " ")
}

// approvalScopeFields parses `k=v;k=v` scope strings back into a map.
// Quoted values (Go %q) are unquoted.
func approvalScopeFields(scope string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(scope, ";") {
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			continue
		}
		key := part[:eq]
		val := part[eq+1:]
		if unq, err := strconvUnquote(val); err == nil {
			val = unq
		}
		out[key] = val
	}
	return out
}

func strconvUnquote(val string) (string, error) {
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		return stringUnquote(val)
	}
	return val, nil
}

// scopeCmd returns the command recorded in an approval scope, plus whether it
// was stored as a generalizable prefix.
func scopeCmd(fields map[string]string) (string, bool) {
	cmd := fields["cmd"]
	if cmd == "" {
		return "", false
	}
	generalizable := fields["generalizable"] != "exact"
	return cmd, generalizable
}

// shellGrantMatches reports whether any granted shell scope covers this call
// via token-prefix matching. Called only when exact-scope lookup failed.
func (a *Agent) shellGrantMatches(approvalScope string) bool {
	req := approvalScopeFields(approvalScope)
	wantCmd, wantGeneralizable := scopeCmd(req)
	if wantCmd == "" || !wantGeneralizable {
		return false
	}
	wantTokens := strings.Fields(wantCmd)

	match := false
	a.sessionGrants.Each(func(scope string) {
		if match || !strings.Contains(scope, "cmd=") {
			return // non-shell or legacy grant, or already matched
		}
		granted := approvalScopeFields(scope)
		if granted["name"] != req["name"] || granted["action"] != req["action"] || granted["risk"] != req["risk"] {
			return
		}
		grantedCmd, generalizable := scopeCmd(granted)
		if !generalizable || grantedCmd == "" {
			return
		}
		gt := strings.Fields(grantedCmd)
		if len(gt) == 0 || len(gt) > len(wantTokens) {
			return
		}
		for i, tok := range gt {
			if wantTokens[i] != tok {
				return
			}
		}
		match = true
	})
	return match
}

// buildApprovalScope assembles the scope string for a tool call. Shell tools
// embed the (possibly generalized) command so grants stay narrowly scoped.
func (a *Agent) buildApprovalScope(tc ai.ToolCall, t tools.Tool) string {
	action, risk := toolApprovalDimensions(tc, t)
	base := func() string {
		return "name=" + quoteScopeValue(tc.Name) + ";action=" + action + ";risk=" + risk
	}
	cmd := shellCommandOf(tc.Name, tc.Args)
	if cmd == "" {
		return base()
	}
	if isGeneralizable(cmd) {
		return base() + ";cmd=" + quoteScopeValue(commandPrefix(cmd)) + ";generalizable=prefix"
	}
	// Dangerous shape: grant matches only this exact command.
	return base() + ";cmd=" + quoteScopeValue(cmd) + ";generalizable=exact"
}

func quoteScopeValue(v string) string {
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(v, "\\", "\\\\"), "\"", "\\\"") + "\""
}

// stringUnquote mirrors strconv.Unquote for the simple double-quoted values
// we produce in quoteScopeValue.
func stringUnquote(s string) (string, error) {
	body := s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			i++
			b.WriteByte(body[i])
			continue
		}
		b.WriteByte(body[i])
	}
	return b.String(), nil
}

// ShellGrantPreview describes, in user-facing words, what pressing
// "always allow" will grant for this call: a reusable command prefix or,
// for dangerous shapes, only this exact command.
func ShellGrantPreview(name string, args map[string]any) string {
	cmd := shellCommandOf(name, args)
	if cmd == "" {
		return "this tool"
	}
	if isGeneralizable(cmd) {
		return fmt.Sprintf("prefix %q", commandPrefix(cmd))
	}
	return "this exact command"
}

// CommandIsDangerous reports whether the call contains constructs that force
// exact-match approval (used by the UI for warning tints).
func CommandIsDangerous(name string, args map[string]any) bool {
	cmd := shellCommandOf(name, args)
	if cmd == "" {
		return false
	}
	return !isGeneralizable(cmd)
}
