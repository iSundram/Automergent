package web

import (
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		// Valid public URLs
		{"https://api.github.com/repos", false},
		{"http://example.com/page", false},
		// Blocked schemes
		{"ftp://example.com", true},
		{"file:///etc/passwd", true},
		{"javascript:alert(1)", true},
		// Blocked cloud metadata hostname
		{"http://169.254.169.254/latest/meta-data/", true},
		{"http://metadata.google.internal", true},
		// Loopback
		{"http://localhost/secret", true},
		{"http://127.0.0.1:8080/admin", true},
	}

	for _, tt := range tests {
		err := validateURL(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateURL(%q): got err=%v, wantErr=%v", tt.url, err, tt.wantErr)
		}
	}
}
