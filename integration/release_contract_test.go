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
		`./scripts/check_version_contract.sh --tag "${GITHUB_REF_NAME}"`,
		`VERSION="$(tr -d '[:space:]' < VERSION)"`,
		`flutree-${VERSION}-macos-${ARCH}.tar.gz`,
		`flutree-${VERSION}-macos-${ARCH}.sha256`,
		`./scripts/verify_macos_binary.sh "dist/$TARBALL" --expected-version "$VERSION"`,
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
	if !strings.Contains(content, "-X main.version=${VERSION}") {
		t.Fatalf("expected package script to inject CLI version from VERSION")
	}
	if !strings.Contains(content, "VERSION_FILE") {
		t.Fatalf("expected package script to resolve VERSION file")
	}
	if strings.Contains(content, "PyInstaller") {
		t.Fatalf("legacy PyInstaller reference still present")
	}
}

func TestVersionContractScriptsAndReleasePleaseConfigExist(t *testing.T) {
	root := projectRoot(t)

	requiredFiles := []string{
		filepath.Join(root, "VERSION"),
		filepath.Join(root, "scripts", "check_version_contract.sh"),
		filepath.Join(root, ".github", "workflows", "release-please.yml"),
		filepath.Join(root, "release-please-config.json"),
		filepath.Join(root, ".release-please-manifest.json"),
	}

	for _, file := range requiredFiles {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("required file missing: %s (%v)", file, err)
		}
	}
}

func TestVersionContractJobRunsInPRWorkflow(t *testing.T) {
	root := projectRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "tests.yml")
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	required := []string{
		`version-contract:`,
		`./scripts/check_version_contract.sh`,
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("tests workflow missing required token %q", token)
		}
	}
}
