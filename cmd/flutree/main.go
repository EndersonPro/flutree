package main

import (
	"errors"
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
	case "add-repo":
		runtime.ExitOnError(runAddRepo(os.Args[2:]))
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
	fs := newFlagSet("list", printListHelp)
	showAll := fs.Bool("all", false, "Include unmanaged Git worktrees.")
	globalScope := fs.Bool("global", false, "List managed worktrees across all registered repositories.")
	if len(args) > 0 && isHelpToken(args[0]) {
		printListHelp()
		return nil
	}
	helpRequested, err := parseFlagSet(fs, args, "Invalid list arguments.", "")
	if err != nil {
		return err
	}
	if helpRequested {
		return nil
	}

	service := app.NewListService(&infraGit.Gateway{}, registry.NewDefault())
	rows, err := service.Run(domain.ListInput{ShowAll: *showAll, GlobalScope: *globalScope})
	if err != nil {
		return err
	}
	ui.RenderList(rows, *showAll)
	return nil
}

func runComplete(args []string) error {
	fs := newFlagSet("complete", printCompleteHelp)
	yes := fs.Bool("yes", false, "Skip interactive confirmation.")
	force := fs.Bool("force", false, "Force worktree removal.")
	nonInteractive := fs.Bool("non-interactive", false, "Disable prompts.")
	if len(args) > 0 && isHelpToken(args[0]) {
		printCompleteHelp()
		return nil
	}
	if len(args) < 1 {
		return domain.NewError(domain.CategoryInput, 2, "Missing worktree name.", "Usage: flutree complete <name> [options]", nil)
	}
	name := args[0]
	helpRequested, err := parseFlagSet(fs, args[1:], "Invalid complete arguments.", "")
	if err != nil {
		return err
	}
	if helpRequested {
		return nil
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
	fs := newFlagSet("create", printCreateHelp)
	branch := fs.String("branch", "", "Target branch name.")
	baseBranch := fs.String("base-branch", "main", "Base branch for worktree creation.")
	scope := fs.String("scope", ".", "Directory scope used to discover Flutter repositories.")
	rootRepo := fs.String("root-repo", "", "Root repository selector.")
	workspace := fs.Bool("workspace", true, "Generate VSCode workspace file.")
	noWorkspace := fs.Bool("no-workspace", false, "Disable VSCode workspace generation.")
	yes := fs.Bool("yes", false, "Acknowledge dry plan automatically in non-interactive mode.")
	nonInteractive := fs.Bool("non-interactive", false, "Disable prompts.")
	reuseExistingBranch := fs.Bool("reuse-existing-branch", false, "Allow non-interactive reuse when target branch already exists.")
	noPackage := fs.Bool("no-package", false, "Create root-only worktree without package selection.")

	var packages multiFlag
	var packageBase multiFlag
	var copyRootFiles multiFlag
	fs.Var(&packages, "package", "Package repository selector. Repeatable.")
	fs.Var(&packageBase, "package-base", "Override package base branch as <selector>=<branch>. Repeatable.")
	fs.Var(&copyRootFiles, "copy-root-file", "Extra root-level file/pattern to copy into each worktree. Repeatable.")

	if len(args) > 0 && isHelpToken(args[0]) {
		printCreateHelp()
		return nil
	}
	if len(args) < 1 {
		return domain.NewError(domain.CategoryInput, 2, "Missing worktree name.", "Usage: flutree create <name> [options]", nil)
	}
	name := args[0]
	helpRequested, err := parseFlagSet(fs, args[1:], "Invalid create arguments.", "")
	if err != nil {
		return err
	}
	if helpRequested {
		return nil
	}
	if *noPackage && len(packages) > 0 && len(packageBase) > 0 {
		return domain.NewError(domain.CategoryInput, 2, "Flag --no-package cannot be combined with --package or --package-base.", "Remove --no-package or remove package flags.", nil)
	}
	if *noPackage && len(packages) > 0 {
		return domain.NewError(domain.CategoryInput, 2, "Flag --no-package cannot be combined with --package.", "Remove --no-package or remove --package.", nil)
	}
	if *noPackage && len(packageBase) > 0 {
		return domain.NewError(domain.CategoryInput, 2, "Flag --no-package cannot be combined with --package-base.", "Remove --no-package or remove --package-base.", nil)
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
		NoPackage:         *noPackage,
		PackageSelectors:  packages,
		PackageBaseBranch: baseMap,
		RootFiles:         copyRootFiles,
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
			NoPackage:         *noPackage,
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
		createInput.NoPackage = wizardResult.NoPackage
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
		NoPackage:         createInput.NoPackage,
		PackageSelectors:  createInput.PackageSelectors,
		PackageBaseBranch: createInput.PackageBaseBranch,
		RootFiles:         createInput.RootFiles,
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

	syncWithRemote := false
	if !*nonInteractive {
		confirmSync, err := promptAdapter.Confirm(
			"Update local branches from origin before creating worktrees?",
			false,
			false,
		)
		if err != nil {
			return err
		}
		syncWithRemote = confirmSync
	}

	result, err := service.Apply(plan, domain.CreateApplyOptions{
		NonInteractive:      createInput.NonInteractive,
		ReuseExistingBranch: *reuseExistingBranch,
		SyncWithRemote:      syncWithRemote,
	})
	if err != nil {
		return err
	}
	ui.RenderCreateSuccess(result)
	return nil
}

func runPubGet(args []string) error {
	fs := newFlagSet("pubget", printPubGetHelp)
	force := fs.Bool("force", false, "Clean cache and remove pubspec.lock before pub get.")
	if len(args) > 0 && isHelpToken(args[0]) {
		printPubGetHelp()
		return nil
	}
	if len(args) < 1 {
		return domain.NewError(domain.CategoryInput, 2, "Missing workspace name.", "Usage: flutree pubget <name> [--force]", nil)
	}

	name := args[0]
	helpRequested, err := parseFlagSet(fs, args[1:], "Invalid pubget arguments.", "")
	if err != nil {
		return err
	}
	if helpRequested {
		return nil
	}

	service := app.NewPubGetService(registry.NewDefault(), &infraPub.Gateway{})
	stopLoading := ui.StartLoading("Running pub get across workspace...")
	result, err := service.Run(domain.PubGetInput{
		Name:  name,
		Force: *force,
	})
	stopLoading(err == nil)
	if err != nil {
		return err
	}

	ui.RenderPubGetSuccess(result)
	return nil
}

func runAddRepo(args []string) error {
	fs := newFlagSet("add-repo", printAddRepoHelp)
	scope := fs.String("scope", ".", "Directory scope used to discover Flutter repositories.")
	nonInteractive := fs.Bool("non-interactive", false, "Disable prompts.")
	var repos multiFlag
	var packageBase multiFlag
	var copyRootFiles multiFlag
	fs.Var(&repos, "repo", "Repository selector to attach. Repeatable.")
	fs.Var(&packageBase, "package-base", "Override package base branch as <selector>=<branch>. Repeatable.")
	fs.Var(&copyRootFiles, "copy-root-file", "Extra root-level file/pattern to copy into each attached worktree. Repeatable.")
	if len(args) > 0 && isHelpToken(args[0]) {
		printAddRepoHelp()
		return nil
	}
	if len(args) < 1 {
		return domain.NewError(domain.CategoryInput, 2, "Missing workspace name.", "Usage: flutree add-repo <workspace> [options]", nil)
	}
	workspaceName := strings.TrimSpace(args[0])
	if workspaceName == "" {
		return domain.NewError(domain.CategoryInput, 2, "Missing workspace name.", "Usage: flutree add-repo <workspace> [options]", nil)
	}

	helpRequested, err := parseFlagSet(fs, args[1:], "Invalid add-repo arguments.", "")
	if err != nil {
		return err
	}
	if helpRequested {
		return nil
	}

	baseMap := map[string]string{}
	for _, entry := range packageBase {
		parts := strings.SplitN(strings.TrimSpace(entry), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return domain.NewError(domain.CategoryInput, 2, "Invalid --package-base format.", "Use --package-base <selector>=<branch>.", nil)
		}
		baseMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	service := app.NewAddRepoService(&infraGit.Gateway{}, registry.NewDefault(), prompt.New())
	result, err := service.Run(domain.AddRepoInput{
		WorkspaceName:     workspaceName,
		ExecutionScope:    *scope,
		RepoSelectors:     repos,
		PackageBaseBranch: baseMap,
		RootFiles:         copyRootFiles,
		NonInteractive:    *nonInteractive,
	})
	if err != nil {
		return err
	}
	ui.RenderAddRepoSuccess(result)
	return nil
}

func runVersion(args []string) error {
	fs := newFlagSet("version", printVersionHelp)
	if len(args) > 0 && isHelpToken(args[0]) {
		printVersionHelp()
		return nil
	}
	helpRequested, err := parseFlagSet(fs, args, "Invalid version arguments.", "Usage: flutree version")
	if err != nil {
		return err
	}
	if helpRequested {
		return nil
	}
	if fs.NArg() != 0 {
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
	fs := newFlagSet("update", printUpdateHelp)
	check := fs.Bool("check", false, "Check whether a brew update is available.")
	apply := fs.Bool("apply", false, "Apply brew update now.")
	if len(args) > 0 && isHelpToken(args[0]) {
		printUpdateHelp()
		return nil
	}
	helpRequested, err := parseFlagSet(fs, args, "Invalid update arguments.", "Usage: flutree update [--check|--apply]")
	if err != nil {
		return err
	}
	if helpRequested {
		return nil
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
	fmt.Println("  flutree add-repo <workspace> [options]")
	fmt.Println("  flutree list [options]")
	fmt.Println("  flutree complete <name> [options]")
	fmt.Println("  flutree pubget <name> [--force]")
	fmt.Println("  flutree update [--check|--apply]")
	fmt.Println("")
	fmt.Println("Tip: Use `flutree <subcommand> --help` to inspect command flags.")
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

func newFlagSet(name string, usage func()) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = usage
	return fs
}

func parseFlagSet(fs *flag.FlagSet, args []string, invalidMessage, hint string) (bool, error) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return false, domain.NewError(domain.CategoryInput, 2, invalidMessage, hint, err)
	}
	return false, nil
}

func isHelpToken(token string) bool {
	switch strings.TrimSpace(token) {
	case "-h", "--help":
		return true
	default:
		return false
	}
}

func printCreateHelp() {
	fmt.Println("Usage:")
	fmt.Println("  flutree create <name> [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --branch <name>                   Target branch name")
	fmt.Println("  --base-branch <name>              Base branch for worktree creation (default: main)")
	fmt.Println("  --scope <path>                    Directory scope used to discover Flutter repositories (default: .)")
	fmt.Println("  --root-repo <selector>            Root repository selector")
	fmt.Println("  --workspace                       Generate VSCode workspace file (default: true)")
	fmt.Println("  --no-workspace                    Disable VSCode workspace generation")
	fmt.Println("  --yes                             Acknowledge dry plan automatically in non-interactive mode")
	fmt.Println("  --non-interactive                 Disable prompts")
	fmt.Println("  --reuse-existing-branch           Allow non-interactive reuse when target branch already exists")
	fmt.Println("  --no-package                      Create root-only worktree without package selection")
	fmt.Println("  --copy-root-file <pattern>        Extra root-level file/pattern to copy into each worktree (repeatable)")
	fmt.Println("  --package <selector>              Package repository selector (repeatable)")
	fmt.Println("  --package-base <sel>=<branch>     Override package base branch (repeatable)")
	fmt.Println("  -h, --help                        Show this help")
}

func printAddRepoHelp() {
	fmt.Println("Usage:")
	fmt.Println("  flutree add-repo <workspace> [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --scope <path>                    Directory scope used to discover Flutter repositories (default: .)")
	fmt.Println("  --repo <selector>                 Repository selector to attach (repeatable)")
	fmt.Println("  --package-base <sel>=<branch>     Override package base branch (repeatable)")
	fmt.Println("  --copy-root-file <pattern>        Extra root-level file/pattern to copy into each attached worktree (repeatable)")
	fmt.Println("  --non-interactive                 Disable prompts")
	fmt.Println("  -h, --help                        Show this help")
}

func printListHelp() {
	fmt.Println("Usage:")
	fmt.Println("  flutree list [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --all                             Include unmanaged Git worktrees")
	fmt.Println("  --global                          List managed worktrees across all registered repositories")
	fmt.Println("  -h, --help                        Show this help")
}

func printCompleteHelp() {
	fmt.Println("Usage:")
	fmt.Println("  flutree complete <name> [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --yes                             Skip interactive confirmation")
	fmt.Println("  --force                           Force worktree removal")
	fmt.Println("  --non-interactive                 Disable prompts")
	fmt.Println("  -h, --help                        Show this help")
}

func printPubGetHelp() {
	fmt.Println("Usage:")
	fmt.Println("  flutree pubget <name> [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --force                           Clean cache and remove pubspec.lock before pub get")
	fmt.Println("  -h, --help                        Show this help")
}

func printUpdateHelp() {
	fmt.Println("Usage:")
	fmt.Println("  flutree update [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --check                           Check whether a brew update is available")
	fmt.Println("  --apply                           Apply brew update now")
	fmt.Println("  -h, --help                        Show this help")
}

func printVersionHelp() {
	fmt.Println("Usage:")
	fmt.Println("  flutree version")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  -h, --help                        Show this help")
}
