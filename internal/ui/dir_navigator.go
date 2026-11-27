package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	navigatorParentEntry = ".."
	navigatorWindowSize  = 12
)

// dirNavigator is a minimal directory browser: it lists ".." plus the
// non-hidden subdirectories of the current path.
type dirNavigator struct {
	path    string
	entries []string
	cursor  int
	errMsg  string
}

func newDirNavigator(start string) dirNavigator {
	abs, err := filepath.Abs(strings.TrimSpace(start))
	if err != nil || strings.TrimSpace(start) == "" {
		if wd, wdErr := os.Getwd(); wdErr == nil {
			abs = wd
		} else {
			abs = string(filepath.Separator)
		}
	}
	n := dirNavigator{path: abs}
	if err := n.open(abs); err != nil {
		n.entries = []string{navigatorParentEntry}
		n.errMsg = fmt.Sprintf("Cannot read %s: %v", abs, err)
	}
	return n
}

// open lists path and, only when it can be read, makes it the current directory.
func (n *dirNavigator) open(path string) error {
	items, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}
		isDir := item.IsDir()
		if !isDir && item.Type()&os.ModeSymlink != 0 {
			if info, statErr := os.Stat(filepath.Join(path, item.Name())); statErr == nil && info.IsDir() {
				isDir = true
			}
		}
		if isDir {
			names = append(names, item.Name())
		}
	}
	sort.Strings(names)
	n.path = path
	n.entries = append([]string{navigatorParentEntry}, names...)
	n.cursor = 0
	n.errMsg = ""
	return nil
}

func (n *dirNavigator) moveUp() {
	if n.cursor > 0 {
		n.cursor--
	}
}

func (n *dirNavigator) moveDown() {
	if n.cursor < len(n.entries)-1 {
		n.cursor++
	}
}

// enter opens the highlighted entry. Unreadable directories keep the current
// location and surface an inline error.
func (n *dirNavigator) enter() {
	if n.cursor < 0 || n.cursor >= len(n.entries) {
		return
	}
	entry := n.entries[n.cursor]
	target := filepath.Join(n.path, entry)
	if entry == navigatorParentEntry {
		target = filepath.Dir(n.path)
	}
	previous := n.path
	if err := n.open(target); err != nil {
		n.errMsg = fmt.Sprintf("Cannot open %s: %v", target, err)
		return
	}
	if entry == navigatorParentEntry {
		// Land on the directory we just left so repeated ".." stays predictable.
		base := filepath.Base(previous)
		for i, name := range n.entries {
			if name == base {
				n.cursor = i
				break
			}
		}
	}
}

func (n dirNavigator) view() string {
	var b strings.Builder
	b.WriteString("Current folder: " + n.path + "\n")
	if n.errMsg != "" {
		b.WriteString(wizardErrorStyle.Render(n.errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	start := 0
	if n.cursor >= navigatorWindowSize {
		start = n.cursor - navigatorWindowSize + 1
	}
	end := start + navigatorWindowSize
	if end > len(n.entries) {
		end = len(n.entries)
	}
	if start > 0 {
		b.WriteString("  ...\n")
	}
	for i := start; i < end; i++ {
		cursor := " "
		if i == n.cursor {
			cursor = ">"
		}
		label := n.entries[i]
		if label != navigatorParentEntry {
			label += "/"
		}
		b.WriteString(fmt.Sprintf("%s %s\n", cursor, label))
	}
	if end < len(n.entries) {
		b.WriteString("  ...\n")
	}
	if len(n.entries) == 1 {
		b.WriteString("  (no subdirectories)\n")
	}
	return b.String()
}
