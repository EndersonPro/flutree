//go:build !windows

package integration_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionCommandGeneratesShellScripts(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "completion", shell)
			if res.code != 0 {
				t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
			}
			if !strings.Contains(res.stdout, "flutree") {
				t.Fatalf("expected completion script mentioning flutree, got: %.200s", res.stdout)
			}
		})
	}
}

func TestUnknownFlagIsInputErrorWithCobraMessage(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "list", "--nope")
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "[input] unknown flag: --nope") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestUnknownFlagWithJSONReturnsJSONErrorEnvelope(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "--json", "list", "--nope")
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	var out struct {
		Error struct {
			Category string `json:"category"`
			Code     int    `json:"code"`
			Message  string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(res.stderr), &out); err != nil {
		t.Fatalf("expected JSON error envelope, got: %s", res.stderr)
	}
	if out.Error.Category != "input" || out.Error.Code != 2 || !strings.Contains(out.Error.Message, "unknown flag: --nope") {
		t.Fatalf("unexpected envelope: %+v", out.Error)
	}
}

func TestUnknownCommandSuggestsClosestMatch(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "lst")
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "Did you mean this?") || !strings.Contains(res.stderr, "list") {
		t.Fatalf("expected suggestion for list, got: %s", res.stderr)
	}
}

func TestFlagsMayPrecedePositionalArguments(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	// Before Cobra, flags placed before <name> were treated as the name.
	// Reaching the root-repo validation proves the flags were parsed.
	res := runCLI(t, bin, repo, testEnv(home), "", "create", "--non-interactive", "--scope", scope, "feature-x")
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "Non-interactive mode requires explicit root repository selection.") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestBareGroupCommandsPrintHelp(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	cases := map[string][]string{
		"root":   {},
		"config": {"config"},
		"mcp":    {"mcp"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			res := runCLI(t, bin, projectRoot(t), testEnv(home), "", args...)
			if res.code != 0 {
				t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
			}
			if !strings.Contains(res.stdout, "Available Commands:") {
				t.Fatalf("expected Cobra help, got: %s", res.stdout)
			}
		})
	}
}

func TestConfigUnknownActionIsInputError(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "delete", "scope.root")
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, `unknown command "delete" for "flutree config"`) {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestLeadingJSONFlagFormatsCommandOutput(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	prefix := runCLI(t, bin, projectRoot(t), testEnv(home), "", "--json", "version")
	suffix := runCLI(t, bin, projectRoot(t), testEnv(home), "", "version", "--json")
	if prefix.code != 0 || suffix.code != 0 {
		t.Fatalf("expected 0, got %d / %d (%s %s)", prefix.code, suffix.code, prefix.stderr, suffix.stderr)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(prefix.stdout), &out); err != nil || out["version"] == "" {
		t.Fatalf("expected JSON version output, got: %s", prefix.stdout)
	}
	if prefix.stdout != suffix.stdout {
		t.Fatalf("expected same output for --json before and after the command: %q vs %q", prefix.stdout, suffix.stdout)
	}
}
