package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseWorkflowTracksTagsAndPublishesExpectedArtifacts(t *testing.T) {
	root := projectRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "release-brew.yml")
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	required := []string{
		`tags:`,
		`- "v*"`,
		`flutree-${VERSION}-macos-${ARCH}.tar.gz`,
		`flutree-${VERSION}-macos-${ARCH}.sha256`,
		`repository: EndersonPro/homebrew-flutree`,
		`HOMEBREW_TAP_TOKEN`,
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("workflow missing required token %q", token)
		}
	}
}

func TestPackageScriptContractUsesGoBuild(t *testing.T) {
	root := projectRoot(t)
	scriptPath := filepath.Join(root, "scripts", "package_macos.sh")
	b, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	if !strings.Contains(content, "go build") {
		t.Fatalf("expected go build in package script")
	}
	if strings.Contains(content, "PyInstaller") {
		t.Fatalf("legacy PyInstaller reference still present")
	}
}
