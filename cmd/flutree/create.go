package main

import (
	"fmt"
	"strings"

	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraConfig "github.com/EndersonPro/flutree/internal/infra/config"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	"github.com/EndersonPro/flutree/internal/infra/prompt"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	infraSDK "github.com/EndersonPro/flutree/internal/infra/sdk"
	"github.com/EndersonPro/flutree/internal/ui"
	"github.com/spf13/cobra"
)

type createOptions struct {
	branch              string
	baseBranch          string
	scope               string
	rootRepo            string
	workspace           bool
	noWorkspace         bool
	yes                 bool
	nonInteractive      bool
	reuseExistingBranch bool
	noPackage           bool
	packages            []string
	packageBase         []string
	copyRootFiles       []string
}

func newCreateCmd() *cobra.Command {
	opts := &createOptions{}
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new worktree with interactive wizard",
		Long: `Create a new worktree with interactive wizard.

The scope (--scope > scope.root > current directory) decides where Flutter
repositories are discovered. In the interactive wizard, if the scope has no
Flutter repositories a folder navigator opens so you can pick the scope folder;
press ctrl+o on the root step to change folder at any time. Navigator keys:
Up/Down (or j/k) move, Enter opens a folder (.. goes up), Tab selects the
current folder as scope, Esc goes up one folder (back or cancel at the folder
it opened at). Folders that are Flutter repos are marked [flutter]; Tab on one
selects it directly as the root repository. On the packages step press ctrl+o
to add packages from any folder: Tab on a Flutter repo adds it selected, Tab on
any other folder adds the repos found under it. Root and package lists filter
as you type. When the scope was chosen in the navigator and differs from
scope.root, the review step offers to save it as the default scope (press s to
toggle, on by default).

--root-repo and --package accept a name, a package name or a path (absolute,
~ or relative to the current directory) to a Flutter repository, even outside
--scope. Every package base branch must exist locally or on origin (default
develop); otherwise create fails before changing anything. If create fails, the
branches it created are deleted again; pre-existing branches are never touched.`,
		Example: `  flutree create feature-login
  flutree create feature-login --scope ~/code --root-repo app --package core --non-interactive --yes`,
		Args: requireName("Missing worktree name.", "Usage: flutree create <name> [flags]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd, args[0], opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.branch, "branch", "", "Target branch name (default: feature/<name>)")
	f.StringVar(&opts.baseBranch, "base-branch", "main", "Base branch for worktree creation")
	f.StringVar(&opts.scope, "scope", ".", "Directory scope for Flutter repo discovery")
	f.StringVar(&opts.rootRepo, "root-repo", "", "Root repository selector: name, package name or path to a Flutter repo (may be outside --scope)")
	f.BoolVar(&opts.workspace, "workspace", true, "Generate VSCode workspace file (default from config workspace.generate)")
	f.BoolVar(&opts.noWorkspace, "no-workspace", false, "Disable VSCode workspace generation")
	f.BoolVar(&opts.yes, "yes", false, "Auto-acknowledge dry plan in non-interactive mode")
	f.BoolVar(&opts.nonInteractive, "non-interactive", false, "Disable prompts")
	f.BoolVar(&opts.reuseExistingBranch, "reuse-existing-branch", false, "Allow non-interactive reuse of existing branch")
	f.BoolVar(&opts.noPackage, "no-package", false, "Create root-only worktree without package selection")
	f.StringArrayVar(&opts.copyRootFiles, "copy-root-file", nil, "Extra root file/pattern to copy (repeatable)")
	f.StringArrayVar(&opts.packages, "package", nil, "Package repository selector: name, package name or path to a Flutter repo (repeatable)")
	f.StringArrayVar(&opts.packageBase, "package-base", nil, "Override package base branch as <selector>=<branch> (repeatable)")
	return cmd
}

func runCreate(cmd *cobra.Command, name string, opts *createOptions) error {
	if opts.noPackage && len(opts.packages) > 0 && len(opts.packageBase) > 0 {
		return domain.NewError(domain.CategoryInput, 2, "Flag --no-package cannot be combined with --package or --package-base.", "Remove --no-package or remove package flags.", nil)
	}
	if opts.noPackage && len(opts.packages) > 0 {
		return domain.NewError(domain.CategoryInput, 2, "Flag --no-package cannot be combined with --package.", "Remove --no-package or remove --package.", nil)
	}
	if opts.noPackage && len(opts.packageBase) > 0 {
		return domain.NewError(domain.CategoryInput, 2, "Flag --no-package cannot be combined with --package-base.", "Remove --no-package or remove --package-base.", nil)
	}
	if opts.nonInteractive && strings.TrimSpace(opts.rootRepo) == "" {
		return domain.NewError(domain.CategoryInput, 2, "Non-interactive mode requires explicit root repository selection.", "Pass --root-repo with a discovered repository name or path.", nil)
	}

	branchName := opts.branch
	if strings.TrimSpace(branchName) == "" {
		branchName = defaultBranchForName(name)
	}

	baseMap, err := parseAssignments(opts.packageBase, "package-base")
	if err != nil {
		return err
	}

	configRepo := infraConfig.NewDefault()
	genWorkspace := opts.workspace && !opts.noWorkspace
	// Explicit flags win; otherwise use the persisted workspace.generate.
	if !cmd.Flags().Changed("workspace") && !cmd.Flags().Changed("no-workspace") {
		if genWorkspace, err = app.NewConfigService(configRepo).GenerateWorkspace(); err != nil {
			return err
		}
	}
	scopeResolver := app.NewScopeResolver(configRepo)
	resolvedScope, err := scopeResolver.Resolve(opts.scope, cmd.Flags().Changed("scope"))
	if err != nil {
		return err
	}

	gitGateway := &infraGit.Gateway{}
	promptAdapter := prompt.New()
	service := app.NewCreateService(gitGateway, registry.NewDefault(), promptAdapter).
		WithSDK(&infraSDK.Gateway{}, app.NewConfigService(configRepo).SDKManager)

	createInput := domain.CreateInput{
		Name:              name,
		Branch:            branchName,
		BaseBranch:        opts.baseBranch,
		ExecutionScope:    resolvedScope,
		RootSelector:      opts.rootRepo,
		NoPackage:         opts.noPackage,
		PackageSelectors:  opts.packages,
		PackageBaseBranch: baseMap,
		RootFiles:         opts.copyRootFiles,
		GenerateWorkspace: genWorkspace,
		Yes:               opts.yes,
		NonInteractive:    opts.nonInteractive,
	}

	applyAfterDryRun := true
	wizardUsed := false
	saveScope := false
	scopeToSave := ""
	if !opts.nonInteractive && ui.SupportsInteractiveWizard() {
		// The wizard scans the scope itself (showing "Scanning ...") and opens
		// its folder navigator when nothing is found. The scope itself was
		// already validated by the resolver.
		persistedScope, err := app.NewConfigService(configRepo).Get("scope.root")
		if err != nil {
			return err
		}

		wizardResult, err := ui.RunCreateWizard(ui.CreateWizardInput{
			Name:              name,
			Branch:            branchName,
			BaseBranch:        opts.baseBranch,
			GenerateWorkspace: genWorkspace,
			RootSelector:      opts.rootRepo,
			NoPackage:         opts.noPackage,
			PackageSelectors:  opts.packages,
			PackageBaseBranch: baseMap,
			Scope:             resolvedScope,
			Discover:          gitGateway.DiscoverFlutterRepos,
			PersistedScope:    persistedScope,
			ScanInitial:       true,
			Inspect: func(path string) (domain.DiscoveredFlutterRepo, bool) {
				repo, ok, _ := gitGateway.InspectFlutterRepo(path)
				return repo, ok
			},
			BaseBranchExists: func(repoRoot, branch string) bool {
				ok, _ := gitGateway.BaseBranchExists(repoRoot, branch)
				return ok
			},
		}, nil)
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
		createInput.Repos = wizardResult.Repos
		if strings.TrimSpace(wizardResult.Scope) != "" {
			createInput.ExecutionScope = wizardResult.Scope
		}
		saveScope = wizardResult.SaveScope
		scopeToSave = wizardResult.Scope
		createInput.NonInteractive = true
		applyAfterDryRun = wizardResult.Apply
		wizardUsed = true
	}

	plan, err := service.BuildDryPlan(createInput)
	if err != nil {
		return err
	}

	// In --json mode stdout carries only the final JSON document.
	if !jsonOutput(cmd) {
		ui.RenderCreateDryPlan(plan)
	}

	if opts.nonInteractive {
		confirmed, err := promptAdapter.ConfirmWithToken("Dry plan ready.", "APPLY", true, opts.yes)
		if err != nil {
			return err
		}
		if !confirmed {
			return domain.NewError(domain.CategoryInput, 2, "Create cancelled before execution.", "Re-run and type APPLY at the final confirmation prompt.", nil)
		}
	} else if !wizardUsed {
		confirmed, err := promptAdapter.ConfirmWithToken("Dry plan ready.", "APPLY", false, false)
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
	if !opts.nonInteractive {
		confirmSync, err := promptAdapter.Confirm("Update local branches from origin before creating worktrees?", false, false)
		if err != nil {
			return err
		}
		syncWithRemote = confirmSync
	}

	result, err := service.Apply(plan, domain.CreateApplyOptions{
		NonInteractive:      createInput.NonInteractive,
		ReuseExistingBranch: opts.reuseExistingBranch,
		SyncWithRemote:      syncWithRemote,
	})
	if err != nil {
		return err
	}
	// The workspace already exists: failing to remember the scope is only a
	// warning, never a failed create.
	if saveScope {
		if _, err := app.NewConfigService(configRepo).Set("scope.root", scopeToSave); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "[warning] Could not save %s as scope.root: %v\n", scopeToSave, err)
		}
	}
	if jsonOutput(cmd) {
		printJSON(cmd.OutOrStdout(), result)
		return nil
	}
	ui.RenderCreateSuccess(result)
	return nil
}
