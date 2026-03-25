package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallDocsIncludeBrewAndGoLocalFlow(t *testing.T) {
	root := projectRoot(t)
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	usage, err := os.ReadFile(filepath.Join(root, "docs", "usage.md"))
	if err != nil {
		t.Fatal(err)
	}

	required := []string{
		"brew tap EndersonPro/flutree",
		"brew install EndersonPro/flutree/flutree",
		"go build -o ./flutree ./cmd/flutree",
		"go test ./...",
	}
	for _, token := range required {
		if !strings.Contains(string(readme), token) && !strings.Contains(string(usage), token) {
			t.Fatalf("expected token %q in README or usage docs", token)
		}
	}
}

func TestArchitectureDocReflectsGoLayout(t *testing.T) {
	root := projectRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "docs", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	required := []string{
		"cmd/flutree",
		"internal/domain",
		"internal/infra",
		"internal/app",
	}
	for _, token := range required {
		if !strings.Contains(text, token) {
			t.Fatalf("architecture doc missing token %q", token)
		}
	}
}
