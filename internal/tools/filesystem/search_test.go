package filesystem

import "testing"

func TestIsBinary(t *testing.T) {
	if !isBinary([]byte{0x00, 0x01}) {
		t.Fatalf("expected binary content")
	}
	if isBinary([]byte("package main\n")) {
		t.Fatalf("expected text content")
	}
}
