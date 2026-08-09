package filesystem

import (
	"os"
	"testing"
)

func TestIsBinaryFile(t *testing.T) {
	// Text file.
	textFile := t.TempDir() + "/hello.go"
	if err := os.WriteFile(textFile, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	isBin, err := isBinaryFile(textFile)
	if err != nil {
		t.Fatalf("isBinaryFile text: %v", err)
	}
	if isBin {
		t.Error("expected text file to be detected as text, got binary")
	}

	// Binary file (contains null bytes).
	binFile := t.TempDir() + "/data.bin"
	if err := os.WriteFile(binFile, []byte{0x00, 0x01, 0x02, 0x03, 0xFF}, 0o644); err != nil {
		t.Fatal(err)
	}
	isBin, err = isBinaryFile(binFile)
	if err != nil {
		t.Fatalf("isBinaryFile binary: %v", err)
	}
	if !isBin {
		t.Error("expected binary file to be detected as binary, got text")
	}
}

func TestAtomicWriteFileFilesystem(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/output.txt"
	content := []byte("hello, world!\n")

	if err := atomicWriteFile(path, content, 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// Ensure no temp files remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "output.txt" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}

func TestReadFileBinaryRejected(t *testing.T) {
	// Binary file should be rejected.
	binFile := t.TempDir() + "/data.bin"
	if err := os.WriteFile(binFile, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &ReadFileTool{}
	result, err := tool.Execute(nil, map[string]any{"path": binFile})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for binary file, got false")
	}
}
