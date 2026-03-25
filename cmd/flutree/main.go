package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	"github.com/EndersonPro/flutree/internal/infra/prompt"
	infraPub "github.com/EndersonPro/flutree/internal/infra/pub"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	infraUpdate "github.com/EndersonPro/flutree/internal/infra/update"
	"github.com/EndersonPro/flutree/internal/runtime"
	"github.com/EndersonPro/flutree/internal/ui"
)

var version = "0.9.0"

var branchNameSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

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
	case "pubget":
		runtime.ExitOnError(runPubGet(os.Args[2:]))
	case "update":
		runtime.ExitOnError(runUpdate(os.Args[2:]))
	case "version", "--version":
		runtime.ExitOnError(runVersion(os.Args[2:]))
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
	reuseExistingBranch := fs.Bool("reuse-existing-branch", false, "Allow non-interactive reuse when target branch already exists.")

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
		branchName = defaultBranchForName(name)
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

	gitGateway := &infraGit.Gateway{}
	promptAdapter := prompt.New()
	service := app.NewCreateService(gitGateway, registry.NewDefault(), promptAdapter)

	createInput := domain.CreateInput{
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
	}

	applyAfterDryRun := true
	wizardUsed := false
	if !*nonInteractive && ui.SupportsInteractiveWizard() {
		repos, err := gitGateway.DiscoverFlutterRepos(*scope)
		if err != nil {
			return err
		}

		wizardResult, err := ui.RunCreateWizard(ui.CreateWizardInput{
			Name:              name,
			Branch:            branchName,
			BaseBranch:        *baseBranch,
			GenerateWorkspace: genWorkspace,
			RootSelector:      *rootRepo,
			PackageSelectors:  packages,
			PackageBaseBranch: baseMap,
		}, repos)
		if err != nil {
			return err
		}
		if wizardResult.Cancelled {
			return domain.NewError(domain.CategoryInput, 2, "Create cancelled before execution.", "Re-run create to open the interactive flow again.", nil)
		}

		createInput.Name = wizardResult.Name
		createInput.Branch = wizardResult.Branch
		createInput.BaseBranch = wizardResult.BaseBranch
		createInput.RootSelector = wizardResult.RootSelector
		createInput.PackageSelectors = wizardResult.PackageSelectors
		createInput.PackageBaseBranch = wizardResult.PackageBaseBranch
		createInput.GenerateWorkspace = wizardResult.GenerateWorkspace
		createInput.NonInteractive = true
		applyAfterDryRun = wizardResult.Apply
		wizardUsed = true
	}

	plan, err := service.BuildDryPlan(domain.CreateInput{
		Name:              createInput.Name,
		Branch:            createInput.Branch,
		BaseBranch:        createInput.BaseBranch,
		ExecutionScope:    createInput.ExecutionScope,
		RootSelector:      createInput.RootSelector,
		PackageSelectors:  createInput.PackageSelectors,
		PackageBaseBranch: createInput.PackageBaseBranch,
		GenerateWorkspace: createInput.GenerateWorkspace,
		Yes:               createInput.Yes,
		NonInteractive:    createInput.NonInteractive,
	})
	if err != nil {
		return err
	}

	ui.RenderCreateDryPlan(plan)

	if *nonInteractive {
		confirmed, err := promptAdapter.ConfirmWithToken(
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
	} else if !wizardUsed {
		confirmed, err := promptAdapter.ConfirmWithToken(
			"Dry plan ready.",
			"APPLY",
			false,
			false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			return domain.NewError(domain.CategoryInput, 2, "Create cancelled before execution.", "Re-run and type APPLY at the final confirmation prompt.", nil)
		}
	} else if !applyAfterDryRun {
		ui.RenderDryRunOnly()
		return nil
	}

	result, err := service.Apply(plan, domain.CreateApplyOptions{
		NonInteractive:      createInput.NonInteractive,
		ReuseExistingBranch: *reuseExistingBranch,
	})
	if err != nil {
		return err
	}
	ui.RenderCreateSuccess(result)
	return nil
}

func runPubGet(args []string) error {
	if len(args) < 1 {
		return domain.NewError(domain.CategoryInput, 2, "Missing workspace name.", "Usage: flutree pubget <name> [--force]", nil)
	}

	name := args[0]
	fs := flag.NewFlagSet("pubget", flag.ContinueOnError)
	force := fs.Bool("force", false, "Clean cache and remove pubspec.lock before pub get.")
	if err := fs.Parse(args[1:]); err != nil {
		return domain.NewError(domain.CategoryInput, 2, "Invalid pubget arguments.", "", err)
	}

	service := app.NewPubGetService(registry.NewDefault(), &infraPub.Gateway{})
	result, err := service.Run(domain.PubGetInput{
		Name:  name,
		Force: *force,
	})
	if err != nil {
		return err
	}

	ui.RenderPubGetSuccess(result)
	return nil
}

func runVersion(args []string) error {
	if len(args) != 0 {
		return domain.NewError(domain.CategoryInput, 2, "Version command does not accept arguments.", "Use 'flutree version' or 'flutree --version'.", nil)
	}
	v := strings.TrimSpace(version)
	if v == "" {
		v = "dev"
	}
	fmt.Println(v)
	return nil
}

func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	check := fs.Bool("check", false, "Check whether a brew update is available.")
	apply := fs.Bool("apply", false, "Apply brew update now.")
	if err := fs.Parse(args); err != nil {
		return domain.NewError(domain.CategoryInput, 2, "Invalid update arguments.", "Usage: flutree update [--check|--apply]", err)
	}
	if fs.NArg() > 0 {
		return domain.NewError(domain.CategoryInput, 2, "Update command does not accept positional arguments.", "Usage: flutree update [--check|--apply]", nil)
	}

	service := app.NewUpdateService(&infraUpdate.BrewGateway{})
	result, err := service.Run(domain.UpdateInput{Check: *check, Apply: *apply})
	if err != nil {
		return err
	}

	if result.Mode == "check" {
		fmt.Printf("mode=check outdated=%t current=%s latest=%s\n", result.Outdated, safeVersion(result.Current), safeVersion(result.Latest))
		return nil
	}

	fmt.Printf("mode=apply outdated=%t current=%s latest=%s\n", result.Outdated, safeVersion(result.Current), safeVersion(result.Latest))
	if strings.TrimSpace(result.UpgradeNotes) != "" {
		fmt.Println(result.UpgradeNotes)
	}
	return nil
}

func printHelp() {
	fmt.Println("flutree - Manage Git worktree lifecycle for Flutter-oriented flows.")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  flutree --version")
	fmt.Println("  flutree version")
	fmt.Println("  flutree create <name> [options]")
	fmt.Println("  flutree list [--all]")
	fmt.Println("  flutree complete <name> [options]")
	fmt.Println("  flutree pubget <name> [--force]")
	fmt.Println("  flutree update [--check|--apply]")
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func defaultBranchForName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = branchNameSanitizer.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		normalized = "workspace"
	}
	return "feature/" + normalized
}

func safeVersion(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}
