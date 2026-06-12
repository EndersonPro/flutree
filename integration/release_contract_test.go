package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleasePleaseWorkflowOrchestratesBrewPublish(t *testing.T) {
	root := projectRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "release-please.yml")
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	required := []string{
		`release_created: ${{ steps.release.outputs.release_created }}`,
		`tag_name: ${{ steps.release.outputs.tag_name }}`,
		`needs: release-please`,
		`if: ${{ needs.release-please.outputs.release_created == 'true' }}`,
		`TAG: ${{ needs.release-please.outputs.tag_name }}`,
		`./scripts/check_version_contract.sh --tag "${TAG}"`,
		`VERSION="$(tr -d '[:space:]' < VERSION)"`,
		`flutree-${VERSION}-macos-${ARCH}.tar.gz`,
		`flutree-${VERSION}-macos-${ARCH}.sha256`,
		`tag_name: ${{ env.TAG }}`,
		`./scripts/verify_macos_binary.sh "dist/$TARBALL" --expected-version "$VERSION"`,
		`repository: EndersonPro/homebrew-flutree`,
		`HOMEBREW_TAP_TOKEN`,
		`if [[ -z "${TAG}" ]]; then`,
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("workflow missing required token %q", token)
		}
	}
}

func TestFallbackReleaseWorkflowIsManualAndRecoveryOnly(t *testing.T) {
	root := projectRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "release-brew.yml")
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	required := []string{
		`workflow_dispatch:`,
		`recovery_ack`,
		`I_UNDERSTAND_FALLBACK`,
		`./scripts/check_version_contract.sh --tag "${TAG}"`,
		`tag_name: ${{ env.TAG }}`,
		`repository: EndersonPro/homebrew-flutree`,
		`HOMEBREW_TAP_TOKEN`,
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("fallback workflow missing required token %q", token)
		}
	}

	forbidden := []string{
		`push:`,
		`tags:`,
		`- "v*"`,
	}
	for _, token := range forbidden {
		if strings.Contains(content, token) {
			t.Fatalf("fallback workflow contains forbidden trigger token %q", token)
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

	configPath := filepath.Join(root, "release-please-config.json")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var config struct {
		Packages map[string]struct {
			ReleaseType string `json:"release-type"`
			VersionFile string `json:"version-file"`
		} `json:"packages"`
	}

	if err := json.Unmarshal(configContent, &config); err != nil {
		t.Fatalf("invalid release-please config: %v", err)
	}

	rootPackage, ok := config.Packages["."]
	if !ok {
		t.Fatalf("release-please config missing root package entry")
	}

	if rootPackage.ReleaseType != "simple" {
		t.Fatalf("expected root release-type=simple, got %q", rootPackage.ReleaseType)
	}

	if rootPackage.VersionFile != "VERSION" {
		t.Fatalf("expected root version-file=VERSION, got %q", rootPackage.VersionFile)
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

func TestGoreleaserReleaseWorkflowExists(t *testing.T) {
	root := projectRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "release.yml")
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	required := []string{
		`push:`,
		`tags:`,
		`- "v*"`,
		`goreleaser/goreleaser-action`,
		`release --clean`,
		`GITHUB_TOKEN`,
		`HOMEBREW_TAP_TOKEN`,
		// goreleaser version must be pinned to a concrete v2 release, not "latest"
		`version: "v2.`,
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("release workflow missing required token %q", token)
		}
	}
}

func TestGoreleaserYamlTokens(t *testing.T) {
	root := projectRoot(t)
	goreleaserPath := filepath.Join(root, ".goreleaser.yaml")
	b, err := os.ReadFile(goreleaserPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	required := []string{
		`CGO_ENABLED=0`,
		`-X main.version=`,
		`scoops`,
		`brews`,
		`EndersonPro/scoop-flutree`,
		`EndersonPro/homebrew-flutree`,
		// format_overrides block must exist AND use format: (not formats:) — guards against v1-style false positive
		`format_overrides:`,
		`format: zip`,
		`checksums.txt`,
		// darwin→macos name mapping must be present (goreleaser renders .Os as "darwin"; Homebrew URLs use "macos")
		`}}macos{{`,
		// release collision prevention: goreleaser must append assets, not replace the release-please release
		`mode: append`,
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf(".goreleaser.yaml missing required token %q", token)
		}
	}
}

func TestWindowsBuildJobExists(t *testing.T) {
	root := projectRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "tests.yml")
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)

	required := []string{
		`windows-build`,
		`GOOS=windows GOARCH=amd64 go build`,
		`go vet`,
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("tests workflow missing windows-build token %q", token)
		}
	}
}
