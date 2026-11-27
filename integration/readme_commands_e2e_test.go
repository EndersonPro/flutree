//go:build !windows

package integration_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// availableCommands parses the "Available Commands:" block of a Cobra help
// output and returns the command names in order.
func availableCommands(t *testing.T, help string) []string {
	t.Helper()
	var names []string
	inBlock := false
	row := regexp.MustCompile(`^\s{2}([a-z][a-z0-9-]*)\s{2,}\S`)
	for _, line := range strings.Split(help, "\n") {
		switch {
		case strings.HasPrefix(line, "Available Commands:"):
			inBlock = true
		case inBlock && strings.TrimSpace(line) == "":
			return names
		case inBlock:
			if m := row.FindStringSubmatch(line); m != nil {
				names = append(names, m[1])
			}
		}
	}
	return names
}

func hasHeading(readme, prefix, name string) bool {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(prefix) + ` ` + regexp.QuoteMeta(name) + `\s*$`)
	return re.MatchString(readme)
}

func TestReadmeCommandsDocumentEveryCommand(t *testing.T) {
	root := projectRoot(t)
	readmePath := filepath.Join(root, "README.md")
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	readme := string(raw)

	scratch := t.TempDir()

	top := runE2E(t, scratch, nil, "--help")
	commands := availableCommands(t, top.stdout)
	if len(commands) == 0 {
		t.Fatalf("no commands parsed from help output:\n%s", top.stdout)
	}
	for _, name := range commands {
		if name == "help" {
			continue
		}
		if !hasHeading(readme, "###", name) {
			t.Errorf("README.md has no `### %s` heading for command %q", name, name)
		}
	}

	mcp := runE2E(t, scratch, nil, "mcp", "--help")
	subs := availableCommands(t, mcp.stdout)
	if len(subs) == 0 {
		t.Fatalf("no mcp subcommands parsed from help output:\n%s", mcp.stdout)
	}
	for _, sub := range subs {
		if !hasHeading(readme, "####", "mcp "+sub) {
			t.Errorf("README.md has no `#### mcp %s` heading for subcommand %q", sub, sub)
		}
	}
}
