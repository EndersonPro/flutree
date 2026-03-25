package ui

import (
	"os"
	"testing"
)

func TestIsTerminalFileFalseForNil(t *testing.T) {
	if isTerminalFile(nil) {
		t.Fatal("expected nil file to be non-terminal")
	}
}

func TestIsTerminalFileFalseForPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	if isTerminalFile(r) {
		t.Fatal("expected pipe reader to be non-terminal")
	}
	if isTerminalFile(w) {
		t.Fatal("expected pipe writer to be non-terminal")
	}
}
