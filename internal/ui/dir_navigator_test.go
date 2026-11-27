package ui

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDirNavigatorMarksFlutterReposAndUpStopsAtStart(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "app")
	sub := filepath.Join(root, "plain", "inner")
	for _, d := range []string{repo, sub} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	n := newDirNavigatorWith(root, func(p string) bool { return p == repo })
	if !strings.Contains(n.view(), "app/ [flutter]") || strings.Contains(n.view(), "plain/ [flutter]") {
		t.Fatalf("only the repo row must be marked:\n%s", n.view())
	}
	if n.up() {
		t.Fatalf("up must not leave the starting folder")
	}
	n.cursor = 2 // plain/
	n.enter()
	n.cursor = 1 // inner/
	n.enter()
	if !n.up() || n.path != filepath.Join(root, "plain") {
		t.Fatalf("up must go one folder up, at %s", n.path)
	}
	if !n.up() || n.path != root {
		t.Fatalf("up must reach the starting folder, at %s", n.path)
	}
	if n.up() {
		t.Fatalf("up must stop at the starting folder")
	}
}

func TestShortenLeftAndWrapText(t *testing.T) {
	if got := shortenLeft("/very/long/path/to/folder", 12); got != "…h/to/folder" || len([]rune(got)) != 12 {
		t.Fatalf("shortenLeft = %q", got)
	}
	if got := shortenLeft("/short", 20); got != "/short" {
		t.Fatalf("shortenLeft must keep short paths, got %q", got)
	}
	for _, line := range strings.Split(wrapText("one two three four five six seven", 10), "\n") {
		if len(line) > 10 {
			t.Fatalf("line %q exceeds the width", line)
		}
	}
}
