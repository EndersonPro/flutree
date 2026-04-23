package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraConfig "github.com/EndersonPro/flutree/internal/infra/config"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	"github.com/EndersonPro/flutree/internal/infra/prompt"
	infraPub "github.com/EndersonPro/flutree/internal/infra/pub"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	infraUpdate "github.com/EndersonPro/flutree/internal/infra/update"
	"github.com/EndersonPro/flutree/internal/runtime"
	"github.com/EndersonPro/flutree/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

var version = "dev"

var branchNameSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func main() {
	if len(os.Args) == 1 {
		printHelp()
		return
	}

	cmd := os.Args[1]
	// Check if --json flag is passed early to use JSON error formatting
	jsonOutput := containsFlag(os.Args, "--json")

	switch cmd {
	case "create":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runCreate(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runCreate(os.Args[2:]))
		}
	case "add-repo":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runAddRepo(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runAddRepo(os.Args[2:]))
		}
	case "list":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runList(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runList(os.Args[2:]))
		}
	case "complete":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runComplete(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runComplete(os.Args[2:]))
		}
	case "pubget":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runPubGet(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runPubGet(os.Args[2:]))
		}
	case "clean":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runClean(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runClean(os.Args[2:]))
		}
	case "update":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runUpdate(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runUpdate(os.Args[2:]))
		}
	case "config":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runConfig(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runConfig(os.Args[2:]))
		}
	case "version", "--version":
		if jsonOutput {
			runtime.ExitOnErrorJSON(runVersion(os.Args[2:]), true)
		} else {
			runtime.ExitOnError(runVersion(os.Args[2:]))
		}
	case "--help", "-h", "help":
		printHelp()
	default:
		runtime.ExitOnError(domain.NewError(domain.CategoryInput, 2, "No such command '"+cmd+"'.", "", nil))
	}
}

func containsFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

type commandResult struct {
	data interface{}
	err  error
	json bool
}

func printJSON(data interface{}, w *os.File) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

func runList(args []string) error {
	fs := newFlagSet("list", printListHelp)
	showAll := fs.Bool("all", false, "Include unmanaged Git worktrees.")
	globalScope := fs.Bool("global", false, "List managed worktrees across all registered repositories.")
	jsonFlag := fs.Bool("json", false, "Output results as JSON.")
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
	if *jsonFlag {
		printJSON(rows, os.Stdout)
		return nil
	}
	ui.RenderList(rows, *showAll)
	return nil
}

func runComplete(args []string) error {
	fs := newFlagSet("complete", printCompleteHelp)
	yes := fs.Bool("yes", false, "Skip interactive confirmation.")
	force := fs.Bool("force", false, "Force worktree removal.")
	nonInteractive := fs.Bool("non-interactive", false, "Disable prompts.")
	jsonFlag := fs.Bool("json", false, "Output results as JSON.")
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
	if *jsonFlag {
		printJSON(result, os.Stdout)
		return nil
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
	jsonFlag := fs.Bool("json", false, "Output results as JSON.")

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
	scopeFlagProvided := wasFlagProvided(fs, "scope")
	configRepo := infraConfig.NewDefault()
	scopeResolver := app.NewScopeResolver(configRepo)
	resolvedScope, err := scopeResolver.Resolve(*scope, scopeFlagProvided)
	if err != nil {
		return err
	}

	gitGateway := &infraGit.Gateway{}
	promptAdapter := prompt.New()
	service := app.NewCreateService(gitGateway, registry.NewDefault(), promptAdapter)

	createInput := domain.CreateInput{
		Name:              name,
		Branch:            branchName,
		BaseBranch:        *baseBranch,
		ExecutionScope:    resolvedScope,
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
		repos, err := gitGateway.DiscoverFlutterRepos(resolvedScope)
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
	if *jsonFlag {
		printJSON(result, os.Stdout)
		return nil
	}
	ui.RenderCreateSuccess(result)
	return nil
}

func runPubGet(args []string) error {
	fs := newFlagSet("pubget", printPubGetHelp)
	force := fs.Bool("force", false, "Clean cache and remove pubspec.lock before pub get.")
	jsonFlag := fs.Bool("json", false, "Output results as JSON.")
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
	if *jsonFlag {
		printJSON(result, os.Stdout)
		return nil
	}
	ui.RenderPubGetSuccess(result)
	return nil
}

func runClean(args []string) error {
	fs := newFlagSet("clean", printCleanHelp)
	force := fs.Bool("force", false, "Remove pubspec.lock after clean.")
	jsonFlag := fs.Bool("json", false, "Output results as JSON.")
	if len(args) > 0 && isHelpToken(args[0]) {
		printCleanHelp()
		return nil
	}
	helpRequested, err := parseFlagSet(fs, args, "Invalid clean arguments.", "")
	if err != nil {
		return err
	}
	if helpRequested {
		return nil
	}

	service := app.NewCleanService(&infraGit.Gateway{}, registry.NewDefault(), &infraPub.Gateway{})
	result, err := service.Run(domain.CleanInput{Force: *force})
	if err != nil {
		return err
	}
	if *jsonFlag {
		printJSON(result, os.Stdout)
		return nil
	}
	ui.RenderCleanSuccess(result)
	return nil
}

func runAddRepo(args []string) error {
	fs := newFlagSet("add-repo", printAddRepoHelp)
	scope := fs.String("scope", ".", "Directory scope used to discover Flutter repositories.")
	syncPolicy := fs.String("sync-policy", string(domain.AddRepoSyncAuto), "Sync policy before worktree creation: auto|always|never.")
	nonInteractive := fs.Bool("non-interactive", false, "Disable prompts.")
	reuseExistingBranch := fs.Bool("reuse-existing-branch", false, "Allow non-interactive reuse when target branch already exists.")
	jsonFlag := fs.Bool("json", false, "Output results as JSON.")
	var repos multiFlag
	var packageBranchSource multiFlag
	var packageBase multiFlag
	var copyRootFiles multiFlag
	fs.Var(&repos, "repo", "Repository selector to attach. Repeatable.")
	fs.Var(&packageBranchSource, "package-branch-source", "Override package target branch as <selector>=<branch>. Repeatable.")
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

	parsedSyncPolicy, err := parseAddRepoSyncPolicy(*syncPolicy)
	if err != nil {
		return err
	}
	scopeFlagProvided := wasFlagProvided(fs, "scope")
	configRepo := infraConfig.NewDefault()
	scopeResolver := app.NewScopeResolver(configRepo)
	resolvedScope, err := scopeResolver.Resolve(*scope, scopeFlagProvided)
	if err != nil {
		return err
	}

	branchSourceMap := map[string]string{}
	for _, entry := range packageBranchSource {
		parts := strings.SplitN(strings.TrimSpace(entry), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return domain.NewError(domain.CategoryInput, 2, "Invalid --package-branch-source format.", "Use --package-branch-source <selector>=<branch>.", nil)
		}
		branchSourceMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
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
	wizardGate := ui.SupportsInteractiveWizard() && !*nonInteractive && len(repos) == 0
	if wizardGate {
		selectionContext, err := service.BuildSelectionContext(app.AddRepoSelectionContextInput{
			WorkspaceName:  workspaceName,
			ExecutionScope: resolvedScope,
		})
		if err != nil {
			return err
		}

		wizardResult, err := ui.RunAddRepoWizard(ui.AddRepoWizardInput{
			WorkspaceName:              workspaceName,
			RootBranch:                 selectionContext.RootBranch,
			InitialSelectors:           repos,
			InitialPackageBranchSource: branchSourceMap,
			InitialPackageBase:         baseMap,
			InitialSyncPolicy:          parsedSyncPolicy,
		}, selectionContext.Candidates)
		if err != nil {
			return err
		}
		if wizardResult.Cancelled || !wizardResult.Apply {
			return domain.NewError(domain.CategoryInput, 2, "Add-repo cancelled before execution.", "Re-run add-repo to open the interactive flow again.", nil)
		}

		repos = append([]string{}, wizardResult.RepoSelectors...)
		branchSourceMap = wizardResult.PackageBranchSource
		baseMap = wizardResult.PackageBaseBranch
		if !(wasFlagProvided(fs, "sync-policy") && parsedSyncPolicy != domain.AddRepoSyncAuto) {
			parsedSyncPolicy = wizardResult.SyncPolicy
		}
		*nonInteractive = true
	}

	result, err := service.Run(domain.AddRepoInput{
		WorkspaceName:       workspaceName,
		ExecutionScope:      resolvedScope,
		RepoSelectors:       repos,
		PackageBranchSource: branchSourceMap,
		PackageBaseBranch:   baseMap,
		RootFiles:           copyRootFiles,
		SyncPolicy:          parsedSyncPolicy,
		ReuseExistingBranch: *reuseExistingBranch,
		NonInteractive:      *nonInteractive,
	})
	if err != nil {
		return err
	}
	if *jsonFlag {
		printJSON(result, os.Stdout)
		return nil
	}
	ui.RenderAddRepoSuccess(result)
	return nil
}

func runVersion(args []string) error {
	fs := newFlagSet("version", printVersionHelp)
	jsonFlag := fs.Bool("json", false, "Output results as JSON.")
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
	} else {
		v = strings.TrimPrefix(v, "v")
		v = strings.TrimPrefix(v, "V")
	}
	if *jsonFlag {
		printJSON(map[string]string{"version": v}, os.Stdout)
		return nil
	}
	fmt.Println(v)
	return nil
}

func runConfig(args []string) error {
	if len(args) == 0 || isHelpToken(args[0]) {
		printConfigHelp()
		return nil
	}

	// Parse --json flag before action to support JSON output for all config operations
	// We need to handle this manually since config has subcommands
	jsonFlag := false
	for i, arg := range args {
		if arg == "--json" {
			jsonFlag = true
			args = append(args[:i], args[i+1:]...)
			break
		}
	}
	if jsonFlag && len(args) == 0 {
		printConfigHelp()
		return nil
	}

	configRepo := infraConfig.NewDefault()
	service := app.NewConfigService(configRepo)

	action := strings.TrimSpace(args[0])
	switch action {
	case "set":
		if len(args) != 3 {
			return domain.NewError(domain.CategoryInput, 2, "Invalid config set arguments.", "Usage: flutree config set scope.root <path>", nil)
		}
		stored, err := service.SetScopeRoot(args[1], args[2])
		if err != nil {
			return err
		}
		if jsonFlag {
			printJSON(map[string]string{"key": args[1], "value": stored}, os.Stdout)
			return nil
		}
		fmt.Println(stored)
		return nil
	case "get":
		if len(args) != 2 {
			return domain.NewError(domain.CategoryInput, 2, "Invalid config get arguments.", "Usage: flutree config get scope.root", nil)
		}
		value, err := service.GetScopeRoot(args[1])
		if err != nil {
			return err
		}
		if jsonFlag {
			printJSON(map[string]string{"key": args[1], "value": value}, os.Stdout)
			return nil
		}
		fmt.Println(value)
		return nil
	default:
		return domain.NewError(domain.CategoryInput, 2, "Invalid config action.", "Use `flutree config set scope.root <path>` or `flutree config get scope.root`.", nil)
	}
}

func runUpdate(args []string) error {
	fs := newFlagSet("update", printUpdateHelp)
	check := fs.Bool("check", false, "Check whether a brew update is available.")
	apply := fs.Bool("apply", false, "Apply brew update now.")
	jsonFlag := fs.Bool("json", false, "Output results as JSON.")
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

	if *jsonFlag {
		printJSON(result, os.Stdout)
		return nil
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
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree"))
	fmt.Println(muted.Render("Manage Git worktree lifecycle for Flutter-oriented flows."))
	fmt.Println("")
	fmt.Println(accent.Render("Commands:"))
	fmt.Println("  " + cmdStyle.Render("create") + " <name> [options]        " + muted.Render("Create a new worktree with interactive wizard"))
	fmt.Println("  " + cmdStyle.Render("add-repo") + " <workspace> [options]  " + muted.Render("Attach repositories to an existing worktree"))
	fmt.Println("  " + cmdStyle.Render("config") + " <set|get> ...            " + muted.Render("Manage persisted CLI configuration"))
	fmt.Println("  " + cmdStyle.Render("list") + " [options]                " + muted.Render("List managed worktrees"))
	fmt.Println("  " + cmdStyle.Render("complete") + " <name> [options]      " + muted.Render("Complete and remove a worktree"))
	fmt.Println("  " + cmdStyle.Render("pubget") + " <name> [--force]        " + muted.Render("Run pub get across workspace packages"))
	fmt.Println("  " + cmdStyle.Render("clean") + " [--force]               " + muted.Render("Clean current managed worktree"))
	fmt.Println("  " + cmdStyle.Render("update") + " [--check|--apply]       " + muted.Render("Check or apply brew updates"))
	fmt.Println("  " + cmdStyle.Render("version") + "                       " + muted.Render("Show version"))
	fmt.Println("")
	fmt.Println(muted.Render("Tip: Use `flutree <subcommand> --help` to inspect command flags."))
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

func wasFlagProvided(fs *flag.FlagSet, name string) bool {
	provided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
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
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree create"))
	fmt.Println(muted.Render("Create a new worktree with interactive wizard."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree create <name> [options]")
	fmt.Println("")
	fmt.Println(accent.Render("Options:"))
	fmt.Println("  " + flagStyle.Render("--branch") + " <name>                   " + muted.Render("Target branch name"))
	fmt.Println("  " + flagStyle.Render("--base-branch") + " <name>              " + muted.Render("Base branch for worktree creation (default: main)"))
	fmt.Println("  " + flagStyle.Render("--scope") + " <path>                    " + muted.Render("Directory scope for Flutter repo discovery (default: .)"))
	fmt.Println("  " + flagStyle.Render("--root-repo") + " <selector>            " + muted.Render("Root repository selector"))
	fmt.Println("  " + flagStyle.Render("--workspace") + "                       " + muted.Render("Generate VSCode workspace file (default: true)"))
	fmt.Println("  " + flagStyle.Render("--no-workspace") + "                    " + muted.Render("Disable VSCode workspace generation"))
	fmt.Println("  " + flagStyle.Render("--yes") + "                             " + muted.Render("Auto-acknowledge dry plan in non-interactive mode"))
	fmt.Println("  " + flagStyle.Render("--non-interactive") + "                 " + muted.Render("Disable prompts"))
	fmt.Println("  " + flagStyle.Render("--reuse-existing-branch") + "           " + muted.Render("Allow non-interactive reuse of existing branch"))
	fmt.Println("  " + flagStyle.Render("--no-package") + "                      " + muted.Render("Create root-only worktree without package selection"))
	fmt.Println("  " + flagStyle.Render("--copy-root-file") + " <pattern>        " + muted.Render("Extra root file/pattern to copy (repeatable)"))
	fmt.Println("  " + flagStyle.Render("--package") + " <selector>              " + muted.Render("Package repository selector (repeatable)"))
	fmt.Println("  " + flagStyle.Render("--package-base") + " <sel>=<branch>     " + muted.Render("Override package base branch (repeatable)"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}

func printAddRepoHelp() {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree add-repo"))
	fmt.Println(muted.Render("Attach repositories to an existing worktree with interactive multiselect + final review when TTY is available."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree add-repo <workspace> [options]")
	fmt.Println("")
	fmt.Println(accent.Render("Options:"))
	fmt.Println("  " + flagStyle.Render("--scope") + " <path>                    " + muted.Render("Directory scope for Flutter repo discovery (default: .)"))
	fmt.Println("  " + flagStyle.Render("--repo") + " <selector>                 " + muted.Render("Repository selector to attach (repeatable). Skips interactive wizard when provided"))
	fmt.Println("  " + flagStyle.Render("--package-branch-source") + " <sel>=<branch>  " + muted.Render("Override package target branch (repeatable)"))
	fmt.Println("  " + flagStyle.Render("--package-base") + " <sel>=<branch>     " + muted.Render("Override package base branch (repeatable)"))
	fmt.Println("  " + flagStyle.Render("--sync-policy") + " <auto|always|never> " + muted.Render("Sync behavior before create (default: auto)"))
	fmt.Println("  " + flagStyle.Render("--reuse-existing-branch") + "           " + muted.Render("Allow non-interactive reuse of existing branch"))
	fmt.Println("  " + flagStyle.Render("--copy-root-file") + " <pattern>        " + muted.Render("Extra root file/pattern to copy (repeatable)"))
	fmt.Println("  " + flagStyle.Render("--non-interactive") + "                 " + muted.Render("Disable interactive wizard/prompts and require deterministic selectors"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}

func printConfigHelp() {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree config"))
	fmt.Println(muted.Render("Manage persisted CLI configuration values."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree config set scope.root <path>")
	fmt.Println("  flutree config get scope.root")
	fmt.Println("")
	fmt.Println(accent.Render("Supported keys:"))
	fmt.Println("  " + flagStyle.Render("scope.root") + "                      " + muted.Render("Default discovery root for create/add-repo when --scope is omitted"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}

func parseAddRepoSyncPolicy(value string) (domain.AddRepoSyncPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(domain.AddRepoSyncAuto):
		return domain.AddRepoSyncAuto, nil
	case string(domain.AddRepoSyncAlways):
		return domain.AddRepoSyncAlways, nil
	case string(domain.AddRepoSyncNever):
		return domain.AddRepoSyncNever, nil
	default:
		return "", domain.NewError(
			domain.CategoryInput,
			2,
			"Invalid --sync-policy value.",
			"Use --sync-policy auto|always|never.",
			nil,
		)
	}
}

func printListHelp() {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree list"))
	fmt.Println(muted.Render("List managed worktrees."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree list [options]")
	fmt.Println("")
	fmt.Println(accent.Render("Options:"))
	fmt.Println("  " + flagStyle.Render("--all") + "                             " + muted.Render("Include unmanaged Git worktrees"))
	fmt.Println("  " + flagStyle.Render("--global") + "                          " + muted.Render("List across all registered repositories"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}

func printCompleteHelp() {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree complete"))
	fmt.Println(muted.Render("Complete and remove a worktree."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree complete <name> [options]")
	fmt.Println("")
	fmt.Println(accent.Render("Options:"))
	fmt.Println("  " + flagStyle.Render("--yes") + "                             " + muted.Render("Skip interactive confirmation"))
	fmt.Println("  " + flagStyle.Render("--force") + "                           " + muted.Render("Force worktree removal"))
	fmt.Println("  " + flagStyle.Render("--non-interactive") + "                 " + muted.Render("Disable prompts"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}

func printPubGetHelp() {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree pubget"))
	fmt.Println(muted.Render("Run pub get across workspace packages."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree pubget <name> [options]")
	fmt.Println("")
	fmt.Println(accent.Render("Options:"))
	fmt.Println("  " + flagStyle.Render("--force") + "                           " + muted.Render("Clean cache and remove pubspec.lock before pub get"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}

func printCleanHelp() {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree clean"))
	fmt.Println(muted.Render("Clean the current managed worktree."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree clean [options]")
	fmt.Println("")
	fmt.Println(accent.Render("Options:"))
	fmt.Println("  " + flagStyle.Render("--force") + "                           " + muted.Render("Also remove pubspec.lock"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}

func printUpdateHelp() {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree update"))
	fmt.Println(muted.Render("Check or apply brew updates."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree update [options]")
	fmt.Println("")
	fmt.Println(accent.Render("Options:"))
	fmt.Println("  " + flagStyle.Render("--check") + "                           " + muted.Render("Check whether a brew update is available"))
	fmt.Println("  " + flagStyle.Render("--apply") + "                           " + muted.Render("Apply brew update now"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}

func printVersionHelp() {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})
	muted := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#A1A1AA"})
	flagStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F4C81", Dark: "#8BC6FF"})

	fmt.Println(accent.Render("🌳 flutree version"))
	fmt.Println(muted.Render("Show version information."))
	fmt.Println("")
	fmt.Println(accent.Render("Usage:"))
	fmt.Println("  flutree version")
	fmt.Println("")
	fmt.Println(accent.Render("Options:"))
	fmt.Println("  " + flagStyle.Render("--json") + "                            " + muted.Render("Output results as JSON"))
	fmt.Println("  " + flagStyle.Render("-h, --help") + "                        " + muted.Render("Show this help"))
}
