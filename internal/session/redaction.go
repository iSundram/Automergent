package session

import (
	"regexp"
)

var secretRegexes = []*regexp.Regexp{
	regexp.MustCompile(`sk-[a-zA-Z0-9\-_]{20,}`),
	regexp.MustCompile(`AIza[0-9A-Za-z-_]{30,40}`),
	regexp.MustCompile(`gh[pousr]_[a-zA-Z0-9]{36,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN (RSA|EC|DSA|PRIVATE) KEY-----`),
	regexp.MustCompile(`bearer\s+[a-zA-Z0-9_\-\.]{20,}`),
}

// RedactText scans text and replaces detected secrets with [REDACTED].
func RedactText(text string) string {
	if text == "" {
		return text
	}
	res := text
	for _, re := range secretRegexes {
		res = re.ReplaceAllString(res, "[REDACTED]")
	}
	return res
}

// RedactSession returns a copy of the session with all message contents and tool results sanitized for secrets.
func RedactSession(sess *Session) *Session {
	if sess == nil {
		return nil
	}
	snap := sess.Snapshot()
	for i := range snap.Messages {
		m := &snap.Messages[i]
		for j := range m.Content {
			part := &m.Content[j]
			if part.Text != "" {
				part.Text = RedactText(part.Text)
			}
			if part.ToolResult != nil {
				part.ToolResult.Content = RedactText(part.ToolResult.Content)
			}
		}
	}
	return snap
}
