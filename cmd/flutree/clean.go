package main

import (
	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	infraPub "github.com/EndersonPro/flutree/internal/infra/pub"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	"github.com/EndersonPro/flutree/internal/ui"
	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean the current managed worktree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service := app.NewCleanService(&infraGit.Gateway{}, registry.NewDefault(), &infraPub.Gateway{})
			result, err := service.Run(domain.CleanInput{Force: force})
			if err != nil {
				return err
			}
			if jsonOutput(cmd) {
				printJSON(cmd.OutOrStdout(), result)
				return nil
			}
			ui.RenderCleanSuccess(result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Also remove pubspec.lock")
	return cmd
}
