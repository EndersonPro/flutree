package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

func RenderCreateDryPlan(plan domain.CreateDryPlan) {
	fmt.Println("=== Dry Plan Preview ===")
	fmt.Printf("root    | %s [%s] | branch=%s | base=%s | %s\n", plan.Root.Repo.Name, plan.Root.Repo.PackageName, plan.Root.Branch, plan.Root.BaseBranch, plan.Root.Path)
	for _, pkg := range plan.Packages {
		fmt.Printf("package | %s [%s] | branch=%s | base=%s | %s\n", pkg.Repo.Name, pkg.Repo.PackageName, pkg.Branch, pkg.BaseBranch, pkg.Path)
	}
	fmt.Println("--- Planned Files ---")
	fmt.Printf("override: %s\n", plan.OverridePath)
	if plan.WorkspacePath != "" {
		fmt.Printf("workspace: %s\n", plan.WorkspacePath)
	}
	fmt.Println("Safety Gate: No git/filesystem side effects have been executed yet.")
}

func RenderCreateSuccess(result domain.CreateResult) {
	fmt.Println("=== Worktree Created ===")
	fmt.Printf("Name: %s\n", result.Record.Name)
	fmt.Printf("Branch: %s\n", result.Record.Branch)
	fmt.Printf("Path: %s\n", result.Record.Path)
	fmt.Printf("Packages: %s\n", strings.Join(result.SelectedPackages, ", "))
	if result.WorkspacePath != "" {
		fmt.Printf("Workspace: %s\n", result.WorkspacePath)
	}
	fmt.Printf("Next: %s\n", result.NextStep)
}

func RenderCompleteSuccess(result domain.CompleteResult) {
	fmt.Println("=== Worktree Completed ===")
	fmt.Printf("Name: %s\n", result.Record.Name)
	fmt.Println("Worktree: removed")
	fmt.Printf("Branch: %s (retained)\n", result.Record.Branch)
}

func RenderList(rows []domain.ListRow, includeUnmanaged bool) {
	if len(rows) == 0 {
		next := "Run `flutree create <name> --branch <branch>` to start one."
		if includeUnmanaged {
			next = "No managed or unmanaged worktrees found in discovered repositories."
		}
		fmt.Println("=== Empty State ===")
		fmt.Println("No managed worktrees found.")
		fmt.Println(next)
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

	fmt.Println("=== Managed Worktrees ===")
	fmt.Println("NAME\tBRANCH\tSTATUS\tPATH")
	for _, row := range rows {
		fmt.Printf("%s\t%s\t%s\t%s\n", row.Name, row.Branch, row.Status, row.Path)
	}
}
