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

	if !regexp.MustCompile(`\|\s*Name\s*\|\s*Branch\s*\|\s*Status\s*\|\s*Path\s*\|`).MatchString(output) {
		t.Fatalf("expected list table header, got: %q", output)
	}
	if !strings.Contains(output, "feature-login (+2 packages)") {
		t.Fatalf("expected package annotation in list row, got: %q", output)
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
