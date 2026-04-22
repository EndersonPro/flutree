package ui

import (
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

var ansiOutputRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func captureStdout(t *testing.T, render func()) string {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	os.Stdout = w
	render()
	_ = w.Close()
	os.Stdout = originalStdout

	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stdout output: %v", err)
	}

	return ansiOutputRegex.ReplaceAllString(string(output), "")
}

func TestRenderCreateDryPlanShowsStructuredSections(t *testing.T) {
	plan := domain.CreateDryPlan{
		Root: domain.PlannedWorktree{
			Repo:       domain.DiscoveredFlutterRepo{Name: "root-app", PackageName: "root_app"},
			Path:       "/worktrees/root-app",
			Branch:     "feature/root",
			BaseBranch: "main",
		},
		Packages: []domain.PlannedWorktree{
			{
				Repo:       domain.DiscoveredFlutterRepo{Name: "core", PackageName: "core"},
				Path:       "/worktrees/core",
				Branch:     "feature/core",
				BaseBranch: "develop",
			},
		},
		OverridePath: "/tmp/.flutree.override",
	}

	output := captureStdout(t, func() { RenderCreateDryPlan(plan) })

	if !strings.Contains(output, "Create Dry Plan") {
		t.Fatalf("expected dry plan header, got: %q", output)
	}
	if !regexp.MustCompile(`\|\s*Role\s*\|\s*Repository\s*\|\s*Package\s*\|\s*Branch\s*\|\s*Base Branch\s*\|\s*Path\s*\|`).MatchString(output) {
		t.Fatalf("expected dry plan table header, got: %q", output)
	}
	if !regexp.MustCompile(`\|\s*package\s*\|\s*core\s*\|\s*core\s*\|\s*feature/core\s*\|\s*develop\s*\|\s*/worktrees/core\s*\|`).MatchString(output) {
		t.Fatalf("expected package row in dry plan table, got: %q", output)
	}
	if !strings.Contains(output, "Planned Files") {
		t.Fatalf("expected planned files section, got: %q", output)
	}
	if !regexp.MustCompile(`\|\s*Type\s*\|\s*Path\s*\|`).MatchString(output) {
		t.Fatalf("expected planned files table header, got: %q", output)
	}
	if !strings.Contains(output, "Safety gate:") {
		t.Fatalf("expected safety gate message, got: %q", output)
	}
}

func TestRenderListEmptyStateIncludesGuidance(t *testing.T) {
	output := captureStdout(t, func() { RenderList(nil, false) })

	if !strings.Contains(output, "Empty State") {
		t.Fatalf("expected empty-state title, got: %q", output)
	}
	if !strings.Contains(output, "No managed worktrees found.") {
		t.Fatalf("expected empty-state message, got: %q", output)
	}
	if !strings.Contains(output, "Run `flutree create <name> --branch <branch>` to start one.") {
		t.Fatalf("expected empty-state next step, got: %q", output)
	}
}

func TestRenderListIncludesPackageAssociationHint(t *testing.T) {
	rows := []domain.ListRow{{
		Name:         "feature-login",
		Branch:       "feature/login",
		Status:       "active",
		Path:         "/tmp/worktrees/feature-login/root/root-app",
		PackageCount: 2,
	}}

	output := captureStdout(t, func() { RenderList(rows, false) })

	if !regexp.MustCompile(`│\s*Name\s*│\s*Branch\s*│\s*Status\s*│\s*Path\s*│`).MatchString(output) {
		t.Fatalf("expected list table header, got: %q", output)
	}
	if !strings.Contains(output, "feature-login (+2 packages)") {
		t.Fatalf("expected package annotation in list row, got: %q", output)
	}
}

func TestRenderListFourColumnsInOrder(t *testing.T) {
	rows := []domain.ListRow{
		{Name: "alpha", Branch: "main", Status: "active", Path: "/tmp/alpha"},
		{Name: "beta", Branch: "feature/beta", Status: "completed", Path: "/tmp/beta"},
	}

	output := captureStdout(t, func() { RenderList(rows, false) })

	// Verify four columns appear in order: Name, Branch, Status, Path
	if !regexp.MustCompile(`│\s*Name\s*│\s*Branch\s*│\s*Status\s*│\s*Path\s*│`).MatchString(output) {
		t.Fatalf("expected four columns in order, got: %q", output)
	}
	if !strings.Contains(output, "alpha") {
		t.Fatalf("expected alpha row, got: %q", output)
	}
	if !strings.Contains(output, "beta") {
		t.Fatalf("expected beta row, got: %q", output)
	}
}

func TestRenderListStatusCellsHaveIconsAndColors(t *testing.T) {
	rows := []domain.ListRow{
		{Name: "a", Branch: "main", Status: "active", Path: "/tmp/a"},
		{Name: "b", Branch: "main", Status: "completed", Path: "/tmp/b"},
		{Name: "c", Branch: "main", Status: "error", Path: "/tmp/c"},
	}

	output := captureStdout(t, func() { RenderList(rows, false) })

	if !strings.Contains(output, "● active") {
		t.Fatalf("expected active status with icon, got: %q", output)
	}
	if !strings.Contains(output, "○ completed") {
		t.Fatalf("expected completed status with icon, got: %q", output)
	}
	if !strings.Contains(output, "✖ error") {
		t.Fatalf("expected error status with icon, got: %q", output)
	}
}

func TestRenderListNarrowTerminalTruncatesPath(t *testing.T) {
	oldDefault := defaultTerminalWidth
	defaultTerminalWidth = 50
	defer func() { defaultTerminalWidth = oldDefault }()

	rows := []domain.ListRow{
		{Name: "feature-login", Branch: "feature/login", Status: "active", Path: "/very/long/path/to/somewhere/over/the/rainbow"},
	}

	output := captureStdout(t, func() { RenderList(rows, false) })

	// Path should be truncated (contains ellipsis)
	if !strings.Contains(output, "…") {
		t.Fatalf("expected truncated path with ellipsis in narrow terminal, got: %q", output)
	}
	// Name and Branch should still be present (not truncated)
	if !strings.Contains(output, "feature-login") {
		t.Fatalf("expected name to be present, got: %q", output)
	}
	if !strings.Contains(output, "feature/login") {
		t.Fatalf("expected branch to be present, got: %q", output)
	}
}

func TestRenderListPipedOutputNoANSI(t *testing.T) {
	rows := []domain.ListRow{
		{Name: "feature-login", Branch: "feature/login", Status: "active", Path: "/tmp/path"},
	}

	// Capture raw output directly to check for ANSI codes
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = w
	RenderList(rows, false)
	_ = w.Close()
	os.Stdout = originalStdout
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stdout output: %v", err)
	}
	rawOutput := string(out)

	if ansiOutputRegex.MatchString(rawOutput) {
		t.Fatalf("expected no ANSI escape codes in piped output, got: %q", rawOutput)
	}
}

func TestRenderPubGetSuccessIncludesForceAndExecutionRows(t *testing.T) {
	result := domain.PubGetResult{
		WorkspaceName: "feature-login",
		Force:         true,
		Packages: []domain.PubGetRepoResult{{
			Name: "feature-login__pkg__core",
			Path: "/tmp/worktrees/feature-login/packages/core",
			Tool: domain.PubToolDart,
			Role: "package",
		}},
		Root: domain.PubGetRepoResult{
			Name: "feature-login",
			Path: "/tmp/worktrees/feature-login/root/root-app",
			Tool: domain.PubToolFlutter,
			Role: "root",
		},
	}

	output := captureStdout(t, func() { RenderPubGetSuccess(result) })

	if !strings.Contains(output, "Pub Get Completed") {
		t.Fatalf("expected pubget header, got: %q", output)
	}
	if !strings.Contains(output, "Mode: force (clean + lock removal)") {
		t.Fatalf("expected force mode line, got: %q", output)
	}
	if !strings.Contains(output, "package | feature-login__pkg__core | tool=dart") {
		t.Fatalf("expected package execution line, got: %q", output)
	}
	if !strings.Contains(output, "root    | feature-login | tool=flutter") {
		t.Fatalf("expected root execution line, got: %q", output)
	}
}

func TestRenderCleanSuccessIncludesForceMetadata(t *testing.T) {
	result := domain.CleanResult{
		Record: domain.RegistryRecord{
			Name: "feature-login",
			Path: "/tmp/worktrees/feature-login/root/root-app",
		},
		Tool:        domain.PubToolFlutter,
		Force:       true,
		LockRemoved: true,
	}

	output := captureStdout(t, func() { RenderCleanSuccess(result) })

	if !strings.Contains(output, "Worktree Clean Completed") {
		t.Fatalf("expected clean header, got: %q", output)
	}
	if !strings.Contains(output, "Mode: force") {
		t.Fatalf("expected force mode line, got: %q", output)
	}
	if !strings.Contains(output, "Lock: pubspec.lock removed") {
		t.Fatalf("expected lock removal line, got: %q", output)
	}
}
