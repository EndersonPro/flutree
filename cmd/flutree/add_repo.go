package main

import (
	"strings"

	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraConfig "github.com/EndersonPro/flutree/internal/infra/config"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	"github.com/EndersonPro/flutree/internal/infra/prompt"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	"github.com/EndersonPro/flutree/internal/ui"
	"github.com/spf13/cobra"
)

type addRepoOptions struct {
	scope               string
	syncPolicy          string
	nonInteractive      bool
	reuseExistingBranch bool
	repos               []string
	packageBranchSource []string
	packageBase         []string
	copyRootFiles       []string
}

func newAddRepoCmd() *cobra.Command {
	opts := &addRepoOptions{}
	cmd := &cobra.Command{
		Use:   "add-repo <workspace>",
		Short: "Attach repositories to an existing worktree",
		Long:  "Attach repositories to an existing worktree with interactive multiselect + final review when TTY is available.",
		Example: `  flutree add-repo feature-login
  flutree add-repo feature-login --repo core --sync-policy never --non-interactive`,
		Args: requireName("Missing workspace name.", "Usage: flutree add-repo <workspace> [flags]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddRepo(cmd, strings.TrimSpace(args[0]), opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.scope, "scope", ".", "Directory scope for Flutter repo discovery")
	f.StringArrayVar(&opts.repos, "repo", nil, "Repository selector to attach (repeatable). Skips interactive wizard when provided")
	f.StringArrayVar(&opts.packageBranchSource, "package-branch-source", nil, "Override package target branch as <selector>=<branch> (repeatable)")
	f.StringArrayVar(&opts.packageBase, "package-base", nil, "Override package base branch as <selector>=<branch> (repeatable)")
	f.StringVar(&opts.syncPolicy, "sync-policy", string(domain.AddRepoSyncAuto), "Sync behavior before create: auto|always|never")
	f.BoolVar(&opts.reuseExistingBranch, "reuse-existing-branch", false, "Allow non-interactive reuse of existing branch")
	f.StringArrayVar(&opts.copyRootFiles, "copy-root-file", nil, "Extra root file/pattern to copy (repeatable)")
	f.BoolVar(&opts.nonInteractive, "non-interactive", false, "Disable interactive wizard/prompts and require deterministic selectors")
	return cmd
}

func runAddRepo(cmd *cobra.Command, workspaceName string, opts *addRepoOptions) error {
	parsedSyncPolicy, err := parseAddRepoSyncPolicy(opts.syncPolicy)
	if err != nil {
		return err
	}
	scopeResolver := app.NewScopeResolver(infraConfig.NewDefault())
	resolvedScope, err := scopeResolver.Resolve(opts.scope, cmd.Flags().Changed("scope"))
	if err != nil {
		return err
	}

	branchSourceMap, err := parseAssignments(opts.packageBranchSource, "package-branch-source")
	if err != nil {
		return err
	}
	baseMap, err := parseAssignments(opts.packageBase, "package-base")
	if err != nil {
		return err
	}

	repos := opts.repos
	nonInteractive := opts.nonInteractive
	service := app.NewAddRepoService(&infraGit.Gateway{}, registry.NewDefault(), prompt.New())
	if ui.SupportsInteractiveWizard() && !nonInteractive && len(repos) == 0 {
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
		if !(cmd.Flags().Changed("sync-policy") && parsedSyncPolicy != domain.AddRepoSyncAuto) {
			parsedSyncPolicy = wizardResult.SyncPolicy
		}
		nonInteractive = true
	}

	result, err := service.Run(domain.AddRepoInput{
		WorkspaceName:       workspaceName,
		ExecutionScope:      resolvedScope,
		RepoSelectors:       repos,
		PackageBranchSource: branchSourceMap,
		PackageBaseBranch:   baseMap,
		RootFiles:           opts.copyRootFiles,
		SyncPolicy:          parsedSyncPolicy,
		ReuseExistingBranch: opts.reuseExistingBranch,
		NonInteractive:      nonInteractive,
	})
	if err != nil {
		return err
	}
	if jsonOutput(cmd) {
		printJSON(cmd.OutOrStdout(), result)
		return nil
	}
	ui.RenderAddRepoSuccess(result)
	return nil
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
		return "", domain.NewError(domain.CategoryInput, 2, "Invalid --sync-policy value.", "Use --sync-policy auto|always|never.", nil)
	}
}
