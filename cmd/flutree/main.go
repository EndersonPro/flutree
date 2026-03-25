package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	"github.com/EndersonPro/flutree/internal/infra/prompt"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	"github.com/EndersonPro/flutree/internal/runtime"
	"github.com/EndersonPro/flutree/internal/ui"
)

func main() {
	if len(os.Args) == 1 {
		printHelp()
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "create":
		runtime.ExitOnError(runCreate(os.Args[2:]))
	case "list":
		runtime.ExitOnError(runList(os.Args[2:]))
	case "complete":
		runtime.ExitOnError(runComplete(os.Args[2:]))
	case "--help", "-h", "help":
		printHelp()
	default:
		runtime.ExitOnError(domain.NewError(domain.CategoryInput, 2, "No such command '"+cmd+"'.", "", nil))
	}
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	showAll := fs.Bool("all", false, "Include unmanaged Git worktrees.")
	if err := fs.Parse(args); err != nil {
		return domain.NewError(domain.CategoryInput, 2, "Invalid list arguments.", "", err)
	}
	service := app.NewListService(&infraGit.Gateway{}, registry.NewDefault())
	rows, err := service.Run(*showAll)
	if err != nil {
		return err
	}
	ui.RenderList(rows, *showAll)
	return nil
}

func runComplete(args []string) error {
	if len(args) < 1 {
		return domain.NewError(domain.CategoryInput, 2, "Missing worktree name.", "Usage: flutree complete <name> [options]", nil)
	}
	name := args[0]
	fs := flag.NewFlagSet("complete", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "Skip interactive confirmation.")
	force := fs.Bool("force", false, "Force worktree removal.")
	nonInteractive := fs.Bool("non-interactive", false, "Disable prompts.")
	if err := fs.Parse(args[1:]); err != nil {
		return domain.NewError(domain.CategoryInput, 2, "Invalid complete arguments.", "", err)
	}
	service := app.NewCompleteService(&infraGit.Gateway{}, registry.NewDefault(), prompt.New())
	result, err := service.Run(domain.CompleteInput{
		Name:           name,
		Yes:            *yes,
		Force:          *force,
		NonInteractive: *nonInteractive,
	})
	if err != nil {
		return err
	}
	ui.RenderCompleteSuccess(result)
	return nil
}

func runCreate(args []string) error {
	if len(args) < 1 {
		return domain.NewError(domain.CategoryInput, 2, "Missing worktree name.", "Usage: flutree create <name> [options]", nil)
	}
	name := args[0]
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	branch := fs.String("branch", "", "Target branch name.")
	baseBranch := fs.String("base-branch", "main", "Base branch for worktree creation.")
	scope := fs.String("scope", ".", "Directory scope used to discover Flutter repositories.")
	rootRepo := fs.String("root-repo", "", "Root repository selector.")
	workspace := fs.Bool("workspace", true, "Generate VSCode workspace file.")
	noWorkspace := fs.Bool("no-workspace", false, "Disable VSCode workspace generation.")
	yes := fs.Bool("yes", false, "Acknowledge dry plan automatically in non-interactive mode.")
	nonInteractive := fs.Bool("non-interactive", false, "Disable prompts.")

	var packages multiFlag
	var packageBase multiFlag
	fs.Var(&packages, "package", "Package repository selector. Repeatable.")
	fs.Var(&packageBase, "package-base", "Override package base branch as <selector>=<branch>. Repeatable.")

	if err := fs.Parse(args[1:]); err != nil {
		return domain.NewError(domain.CategoryInput, 2, "Invalid create arguments.", "", err)
	}
	if *nonInteractive && strings.TrimSpace(*rootRepo) == "" {
		return domain.NewError(domain.CategoryInput, 2, "Non-interactive mode requires explicit root repository selection.", "Pass --root-repo with a discovered repository name or path.", nil)
	}

	branchName := *branch
	if strings.TrimSpace(branchName) == "" {
		branchName = "feature/" + strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
	}

	baseMap := map[string]string{}
	for _, entry := range packageBase {
		parts := strings.SplitN(strings.TrimSpace(entry), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return domain.NewError(domain.CategoryInput, 2, "Invalid --package-base format.", "Use --package-base <selector>=<branch>.", nil)
		}
		baseMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	genWorkspace := *workspace && !*noWorkspace

	service := app.NewCreateService(&infraGit.Gateway{}, registry.NewDefault(), prompt.New())
	plan, err := service.BuildDryPlan(domain.CreateInput{
		Name:              name,
		Branch:            branchName,
		BaseBranch:        *baseBranch,
		ExecutionScope:    *scope,
		RootSelector:      *rootRepo,
		PackageSelectors:  packages,
		PackageBaseBranch: baseMap,
		GenerateWorkspace: genWorkspace,
		Yes:               *yes,
		NonInteractive:    *nonInteractive,
	})
	if err != nil {
		return err
	}

	ui.RenderCreateDryPlan(plan)

	confirmed, err := prompt.New().ConfirmWithToken(
		"Dry plan ready.",
		"APPLY",
		*nonInteractive,
		*yes && *nonInteractive,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		return domain.NewError(domain.CategoryInput, 2, "Create cancelled before execution.", "Re-run and type APPLY at the final confirmation prompt.", nil)
	}

	result, err := service.Apply(plan)
	if err != nil {
		return err
	}
	ui.RenderCreateSuccess(result)
	return nil
}

func printHelp() {
	fmt.Println("flutree - Manage Git worktree lifecycle for Flutter-oriented flows.")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  flutree create <name> [options]")
	fmt.Println("  flutree list [--all]")
	fmt.Println("  flutree complete <name> [options]")
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
