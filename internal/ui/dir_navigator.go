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
	flutterMarker        = " [flutter]"
)

// dirNavigator is a minimal directory browser: it lists ".." plus the
// non-hidden subdirectories of the current path. An optional inspector marks
// the folders that are Flutter repositories.
type dirNavigator struct {
	path    string
	start   string // the folder the navigator opened at
	entries []string
	cursor  int
	errMsg  string
	width   int // terminal width; 0 means unknown (no shortening)

	inspect       func(path string) bool
	repoRows      map[string]bool
	currentIsRepo bool
}

func newDirNavigator(start string) dirNavigator {
	return newDirNavigatorWith(start, nil)
}

// newDirNavigatorWith builds a navigator whose inspect func reports whether a
// folder is a Flutter repository (nil disables the markers).
func newDirNavigatorWith(start string, inspect func(path string) bool) dirNavigator {
	abs, err := filepath.Abs(strings.TrimSpace(start))
	if err != nil || strings.TrimSpace(start) == "" {
		if wd, wdErr := os.Getwd(); wdErr == nil {
			abs = wd
		} else {
			abs = string(filepath.Separator)
		}
	}
	n := dirNavigator{path: abs, start: abs, inspect: inspect}
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
	n.repoRows = map[string]bool{}
	n.currentIsRepo = false
	if n.inspect != nil {
		n.currentIsRepo = n.inspect(path)
		for _, name := range names {
			if n.inspect(filepath.Join(path, name)) {
				n.repoRows[name] = true
			}
		}
	}
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
	if entry == navigatorParentEntry {
		n.goParent()
		return
	}
	target := filepath.Join(n.path, entry)
	if err := n.open(target); err != nil {
		n.errMsg = fmt.Sprintf("Cannot open %s: %v", target, err)
	}
}

// goParent opens the parent folder and lands the cursor on the folder just
// left, so repeated ".." stays predictable.
func (n *dirNavigator) goParent() {
	previous := n.path
	target := filepath.Dir(n.path)
	if err := n.open(target); err != nil {
		n.errMsg = fmt.Sprintf("Cannot open %s: %v", target, err)
		return
	}
	base := filepath.Base(previous)
	for i, name := range n.entries {
		if name == base {
			n.cursor = i
			break
		}
	}
}

// belowStart reports whether the current folder is strictly inside the folder
// the navigator opened at.
func (n dirNavigator) belowStart() bool {
	rel, err := filepath.Rel(n.start, n.path)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// up goes one folder up while below the starting folder and reports whether it did.
func (n *dirNavigator) up() bool {
	if !n.belowStart() {
		return false
	}
	n.goParent()
	return true
}

func (n dirNavigator) view() string {
	var b strings.Builder
	const prefix = "Current folder: "
	marker := ""
	if n.currentIsRepo {
		marker = flutterMarker
	}
	b.WriteString(prefix + shortenLeft(n.path, n.width-len(prefix)-len(marker)) + marker + "\n")
	if n.errMsg != "" {
		b.WriteString(wizardErrorStyle.Render(wrapText(n.errMsg, n.width)))
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
			if n.repoRows[n.entries[i]] {
				label += flutterMarker
			}
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

// shortenLeft keeps the tail of value within max runes, replacing the cut head
// with "…". A max that is not positive (unknown width) leaves value untouched.
func shortenLeft(value string, max int) string {
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max == 1 {
		return "…"
	}
	return "…" + string(runes[len(runes)-(max-1):])
}

// wrapText breaks text on spaces so no line exceeds width runes. A width that
// is not positive (unknown) leaves the text untouched; a word longer than the
// width stays whole on its own line.
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len([]rune(line))+1+len([]rune(word)) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
