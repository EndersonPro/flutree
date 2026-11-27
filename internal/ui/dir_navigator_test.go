package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirNavigatorListsSortedVisibleDirectoriesWithParentFirst(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"zeta", "alpha", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	n := newDirNavigator(root)
	want := []string{"..", "alpha", "zeta"}
	if len(n.entries) != len(want) {
		t.Fatalf("entries = %v, want %v", n.entries, want)
	}
	for i := range want {
		if n.entries[i] != want[i] {
			t.Fatalf("entries = %v, want %v", n.entries, want)
		}
	}
}

func TestDirNavigatorEnterOpensChildAndParent(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}

	n := newDirNavigator(root)
	n.moveDown()
	n.enter()
	if n.path != child {
		t.Fatalf("path = %q, want %q", n.path, child)
	}
	n.enter() // ".." is highlighted first
	if n.path != root {
		t.Fatalf("path = %q, want %q", n.path, root)
	}
}

func TestDirNavigatorUnreadableDirectoryShowsInlineError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	n := newDirNavigator(root)
	n.moveDown()
	n.enter()
	if n.errMsg == "" {
		t.Fatalf("expected an inline error for an unreadable directory")
	}
	if n.path != root {
		t.Fatalf("navigator must stay in %q, got %q", root, n.path)
	}
}
