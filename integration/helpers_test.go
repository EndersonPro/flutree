package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
	return root
}
