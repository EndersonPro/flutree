package main

import (
	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	"github.com/EndersonPro/flutree/internal/infra/prompt"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	"github.com/EndersonPro/flutree/internal/ui"
	"github.com/spf13/cobra"
)

func newCompleteCmd() *cobra.Command {
	var yes, force, nonInteractive bool
	cmd := &cobra.Command{
		Use:   "complete <name>",
		Short: "Complete and remove a worktree",
		Args:  requireName("Missing worktree name.", "Usage: flutree complete <name> [flags]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			service := app.NewCompleteService(&infraGit.Gateway{}, registry.NewDefault(), prompt.New())
			result, err := service.Run(domain.CompleteInput{
				Name:           args[0],
				Yes:            yes,
				Force:          force,
				NonInteractive: nonInteractive,
			})
			if err != nil {
				return err
			}
			if jsonOutput(cmd) {
				printJSON(cmd.OutOrStdout(), result)
				return nil
			}
			ui.RenderCompleteSuccess(result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip interactive confirmation")
	cmd.Flags().BoolVar(&force, "force", false, "Force worktree removal")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Disable prompts")
	return cmd
}
