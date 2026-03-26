package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

var (
	outputHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(uiAccentColor).Padding(0, 1)
	outputBodyStyle   = lipgloss.NewStyle().PaddingLeft(2)
	outputMutedStyle  = lipgloss.NewStyle().PaddingLeft(2).Foreground(uiMutedColor)
)

func RenderCreateDryPlan(plan domain.CreateDryPlan) {
	fmt.Println(outputHeaderStyle.Render("Create Dry Plan"))
	rows := [][]string{{
		"root",
		plan.Root.Repo.Name,
		plan.Root.Repo.PackageName,
		plan.Root.Branch,
		plan.Root.BaseBranch,
		plan.Root.Path,
	}}
	for _, pkg := range plan.Packages {
		rows = append(rows, []string{"package", pkg.Repo.Name, pkg.Repo.PackageName, pkg.Branch, pkg.BaseBranch, pkg.Path})
	}
	fmt.Println(outputBodyStyle.Render(renderTable([]string{"Role", "Repository", "Package", "Branch", "Base Branch", "Path"}, rows)))
	fmt.Println(outputHeaderStyle.Render("Planned Files"))
	fileRows := [][]string{{"Override", plan.OverridePath}}
	if plan.WorkspacePath != "" {
		fileRows = append(fileRows, []string{"Workspace", plan.WorkspacePath})
	}
	fmt.Println(outputBodyStyle.Render(renderTable([]string{"Type", "Path"}, fileRows)))
	fmt.Println(outputMutedStyle.Render("Safety gate: No git/filesystem side effects have been executed yet."))
}

func RenderCreateSuccess(result domain.CreateResult) {
	fmt.Println(outputHeaderStyle.Render("Worktree Created"))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Name: %s", result.Record.Name)))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Branch: %s", result.Record.Branch)))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Path: %s", result.Record.Path)))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Packages: %s", strings.Join(result.SelectedPackages, ", "))))
	if result.WorkspacePath != "" {
		fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Workspace: %s", result.WorkspacePath)))
	}
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Next: %s", result.NextStep)))
}

func RenderDryRunOnly() {
	fmt.Println(outputHeaderStyle.Render("Dry Plan Completed"))
	fmt.Println(outputMutedStyle.Render("No filesystem or git changes were applied."))
}

func RenderCompleteSuccess(result domain.CompleteResult) {
	fmt.Println(outputHeaderStyle.Render("Worktree Completed"))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Name: %s", result.Record.Name)))
	fmt.Println(outputBodyStyle.Render("Worktree: removed"))
	if result.StaleCleaned {
		fmt.Println(outputBodyStyle.Render("Registry: stale entry cleaned (missing path)"))
	}
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Branch: %s (retained)", result.Record.Branch)))
}

func RenderPubGetSuccess(result domain.PubGetResult) {
	fmt.Println(outputHeaderStyle.Render("Pub Get Completed"))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Workspace: %s", result.WorkspaceName)))
	if result.Force {
		fmt.Println(outputBodyStyle.Render("Mode: force (clean + lock removal)"))
	}
	for _, pkg := range result.Packages {
		fmt.Println(outputBodyStyle.Render(fmt.Sprintf("package | %s | tool=%s | %s", pkg.Name, pkg.Tool, pkg.Path)))
	}
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("root    | %s | tool=%s | %s", result.Root.Name, result.Root.Tool, result.Root.Path)))
}

func RenderAddRepoSuccess(result domain.AddRepoResult) {
	fmt.Println(outputHeaderStyle.Render("Repository Attached"))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Workspace: %s", result.WorkspaceName)))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Branch: %s", result.SelectedBranch)))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Added repos: %s", strings.Join(result.AddedRepos, ", "))))
	fmt.Println(outputBodyStyle.Render(fmt.Sprintf("Override updated: %s", result.OverridePath)))
}

func RenderList(rows []domain.ListRow, includeUnmanaged bool) {
	if len(rows) == 0 {
		next := "Run `flutree create <name> --branch <branch>` to start one."
		if includeUnmanaged {
			next = "No managed or unmanaged worktrees found in discovered repositories."
		}
		fmt.Println(outputHeaderStyle.Render("Empty State"))
		fmt.Println(outputBodyStyle.Render("No managed worktrees found."))
		fmt.Println(outputBodyStyle.Render(next))
		return
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Status != rows[j].Status {
			return rows[i].Status < rows[j].Status
		}
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].Path < rows[j].Path
	})

	fmt.Println(outputHeaderStyle.Render("Managed Worktrees"))
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		displayName := row.Name
		if row.PackageCount > 0 {
			displayName = fmt.Sprintf("%s (+%d packages)", row.Name, row.PackageCount)
		}
		tableRows = append(tableRows, []string{displayName, row.Branch, row.Status, row.Path})
	}
	fmt.Println(outputBodyStyle.Render(renderTable([]string{"Name", "Branch", "Status", "Path"}, tableRows)))
}
