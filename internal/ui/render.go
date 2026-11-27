package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

func RenderCreateDryPlan(plan domain.CreateDryPlan) {
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
	worktreeTable := renderTable([]string{"Role", "Repository", "Package", "Branch", "Base Branch", "Path"}, rows)

	fileRows := [][]string{}
	if plan.OverridePath != "" {
		fileRows = append(fileRows, []string{"Override", plan.OverridePath})
	}
	for _, po := range plan.PackageOverrides {
		if po.Content != "" {
			fileRows = append(fileRows, []string{"Package override", po.Path})
		}
	}
	if plan.WorkspacePath != "" {
		fileRows = append(fileRows, []string{"Workspace", plan.WorkspacePath})
	}
	filesTable := renderTable([]string{"Type", "Path"}, fileRows)

	planHeader := uiIconStyle.Render("📋") + uiHeaderStyle.Render("Create Dry Plan")
	filesHeader := uiIconStyle.Render("📁") + uiSubheaderStyle.Render("Planned Files")
	safetyMsg := uiIconStyle.Render("⚡") + uiMutedStyle.Render("Safety gate: No git/filesystem side effects have been executed yet.")

	var b strings.Builder
	b.WriteString(planHeader)
	b.WriteString("\n")
	b.WriteString(uiBodyStyle.Render(worktreeTable))
	b.WriteString("\n")
	if len(fileRows) > 0 {
		b.WriteString(filesHeader)
		b.WriteString("\n")
		b.WriteString(uiBodyStyle.Render(filesTable))
		b.WriteString("\n")
	}
	b.WriteString(safetyMsg)
	fmt.Println(b.String())
}

func RenderCreateSuccess(result domain.CreateResult) {
	var b strings.Builder
	b.WriteString(KeyValue("Name", result.Record.Name))
	b.WriteString("\n")
	b.WriteString(KeyValue("Branch", result.Record.Branch))
	b.WriteString("\n")
	b.WriteString(KeyValue("Path", result.Record.Path))
	b.WriteString("\n")
	b.WriteString(KeyValue("Packages", strings.Join(result.SelectedPackages, ", ")))
	if result.WorkspacePath != "" {
		b.WriteString("\n")
		b.WriteString(KeyValue("Workspace", result.WorkspacePath))
	}
	writePackageOverrideSummary(&b, result.PackageOverrides)
	writeSDKSummary(&b, result.SDKSetup)
	b.WriteString("\n")
	b.WriteString(KeyValue("Next", result.NextStep))

	header := uiIconStyle.Render("✅") + uiSuccessHeader.Render("Worktree Created")
	fmt.Println(header)
	fmt.Println(SuccessBox(b.String()))
	warnSDKFailures(result.SDKSetup)
}

// writeSDKSummary adds one "SDK" line listing the worktrees whose SDK manager
// ran successfully ("<repo>: <manager> <version>").
func writeSDKSummary(b *strings.Builder, results []domain.SDKSetupResult) {
	configured := []string{}
	for _, r := range results {
		if r.Status == "configured" {
			configured = append(configured, fmt.Sprintf("%s: %s %s", r.Repo, r.Manager, r.Version))
		}
	}
	if len(configured) == 0 {
		return
	}
	b.WriteString("\n")
	b.WriteString(KeyValue("SDK", strings.Join(configured, ", ")))
}

// warnSDKFailures prints one stderr warning per worktree whose SDK manager
// failed; the worktree itself was created and stays usable.
func warnSDKFailures(results []domain.SDKSetupResult) {
	for _, r := range results {
		if r.Status != "failed" {
			continue
		}
		fmt.Fprintf(os.Stderr, "[warning] SDK setup failed for %s (%s %s): %s\n", r.Repo, r.Manager, r.Version, r.Message)
		fmt.Fprintf(os.Stderr, "Hint: run the SDK manager manually in %s\n", r.Path)
	}
}

func RenderDryRunOnly() {
	var b strings.Builder
	b.WriteString(uiMutedStyle.Render("No filesystem or git changes were applied."))

	header := uiIconStyle.Render("ℹ️") + uiInfoHeader.Render("Dry Plan Completed")
	fmt.Println(header)
	fmt.Println(InfoBox(b.String()))
}

func RenderCompleteSuccess(result domain.CompleteResult) {
	var b strings.Builder
	b.WriteString(KeyValue("Name", result.Record.Name))
	b.WriteString("\n")
	b.WriteString(KeyValue("Worktree", "removed"))
	if result.StaleCleaned {
		b.WriteString("\n")
		b.WriteString(KeyValue("Registry", "stale entry cleaned (missing path)"))
	}
	b.WriteString("\n")
	b.WriteString(KeyValue("Branch", result.Record.Branch+" (retained)"))

	header := uiIconStyle.Render("✅") + uiSuccessHeader.Render("Worktree Completed")
	fmt.Println(header)
	fmt.Println(SuccessBox(b.String()))
}

func RenderPubGetSuccess(result domain.PubGetResult) {
	var b strings.Builder
	b.WriteString(KeyValue("Workspace", result.WorkspaceName))
	if result.Force {
		b.WriteString("\n")
		b.WriteString(KeyValue("Mode", "force (clean + lock removal)"))
	}
	b.WriteString("\n")

	for _, pkg := range result.Packages {
		b.WriteString(uiBodyStyle.Render(
			lipgloss.NewStyle().Foreground(uiInfoColor).Render("•") + " " +
				lipgloss.NewStyle().Render(fmt.Sprintf("package | %s | tool=%s | %s", pkg.Name, pkg.Tool, pkg.Path)),
		))
		b.WriteString("\n")
	}
	b.WriteString(uiBodyStyle.Render(
		lipgloss.NewStyle().Foreground(uiSuccessColor).Render("★") + " " +
			lipgloss.NewStyle().Render(fmt.Sprintf("root    | %s | tool=%s | %s", result.Root.Name, result.Root.Tool, result.Root.Path)),
	))

	header := uiIconStyle.Render("✅") + uiSuccessHeader.Render("Pub Get Completed")
	fmt.Println(header)
	fmt.Println(SuccessBox(b.String()))
}

func RenderCleanSuccess(result domain.CleanResult) {
	var b strings.Builder
	b.WriteString(KeyValue("Worktree", result.Record.Name))
	b.WriteString("\n")
	b.WriteString(KeyValue("Path", result.Record.Path))
	b.WriteString("\n")
	b.WriteString(KeyValue("Tool", string(result.Tool)))
	if result.Force {
		b.WriteString("\n")
		b.WriteString(KeyValue("Mode", "force"))
	}
	if result.LockRemoved {
		b.WriteString("\n")
		b.WriteString(KeyValue("Lock", "pubspec.lock removed"))
	}

	header := uiIconStyle.Render("✅") + uiSuccessHeader.Render("Worktree Clean Completed")
	fmt.Println(header)
	fmt.Println(SuccessBox(b.String()))
}

// writePackageOverrideSummary lists the package override files and any skip note.
func writePackageOverrideSummary(b *strings.Builder, results []domain.PackageOverrideResult) {
	for _, r := range results {
		b.WriteString("\n")
		line := r.Status + " " + r.Path
		if r.Note != "" {
			line += " (" + r.Note + ")"
		}
		b.WriteString(KeyValue("Package override "+r.Repo, line))
	}
}

func RenderAddRepoSuccess(result domain.AddRepoResult) {
	var b strings.Builder
	b.WriteString(KeyValue("Workspace", result.WorkspaceName))
	b.WriteString("\n")
	b.WriteString(KeyValue("Branch", result.SelectedBranch))
	b.WriteString("\n")
	b.WriteString(KeyValue("Added repos", strings.Join(result.AddedRepos, ", ")))
	b.WriteString("\n")
	b.WriteString(KeyValue("Override updated", result.OverridePath))
	writePackageOverrideSummary(&b, result.PackageOverrides)
	writeSDKSummary(&b, result.SDKSetup)

	header := uiIconStyle.Render("✅") + uiSuccessHeader.Render("Repository Attached")
	fmt.Println(header)
	fmt.Println(SuccessBox(b.String()))
	warnSDKFailures(result.SDKSetup)
}

func RenderList(rows []domain.ListRow, includeUnmanaged bool) {
	if len(rows) == 0 {
		next := "Run `flutree create <name> --branch <branch>` to start one."
		if includeUnmanaged {
			next = "No managed or unmanaged worktrees found in discovered repositories."
		}

		var b strings.Builder
		b.WriteString(uiMutedStyle.Render("No managed worktrees found."))
		b.WriteString("\n")
		b.WriteString(uiMutedStyle.Render(next))

		header := uiIconStyle.Render("📭") + uiHeaderStyle.Render("Empty State")
		fmt.Println(header)
		fmt.Println(InfoBox(b.String()))
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

	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		displayName := row.Name
		if row.PackageCount > 0 {
			displayName = fmt.Sprintf("%s (+%d packages)", row.Name, row.PackageCount)
		}
		tableRows = append(tableRows, []string{displayName, row.Branch, row.Status, row.Path})
	}

	header := uiIconStyle.Render("🌳") + uiHeaderStyle.Render("Managed Worktrees")
	fmt.Println(header)
	fmt.Println(uiBodyStyle.Render(renderStyledTable([]string{"Name", "Branch", "Status", "Path"}, tableRows, rows)))
}
