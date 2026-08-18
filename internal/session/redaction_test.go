package session

import (
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
)

func TestRedactText(t *testing.T) {
	input := "Here is my secret sk-proj-1234567890abcdefghijklmnopqrstuvwxyz and token AIzaSyAbCdEfGhIjKlMnOpQrStUvWxYz123456 and bearer secret_token_1234567890abcdef."
	redacted := RedactText(input)

	if testing.Verbose() {
		t.Logf("Redacted: %s", redacted)
	}

	if containsString(redacted, "sk-proj-1234567890abcdefghijklmnopqrstuvwxyz") ||
		containsString(redacted, "AIzaSyAbCdEfGhIjKlMnOpQrStUvWxYz123456") {
		t.Errorf("secret not redacted: %q", redacted)
	}
}

func TestRedactSession(t *testing.T) {
	sess := New()
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, "My API key is AIzaSyFakeKeyForTestingPurpose1234567890abcdef"))

	redactedSess := RedactSession(sess)
	msgText := redactedSess.Messages[0].TextContent()

	if containsString(msgText, "AIzaSyFakeKeyForTestingPurpose1234567890abcdef") {
		t.Errorf("session message not redacted: %q", msgText)
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || stringContainsHelper(s, sub))
}

func stringContainsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
