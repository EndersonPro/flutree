package main

import (
	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	"github.com/EndersonPro/flutree/internal/ui"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var showAll, globalScope bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed worktrees",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			service := app.NewListService(&infraGit.Gateway{}, registry.NewDefault())
			rows, err := service.Run(domain.ListInput{ShowAll: showAll, GlobalScope: globalScope})
			if err != nil {
				return err
			}
			if jsonOutput(cmd) {
				printJSON(cmd.OutOrStdout(), rows)
				return nil
			}
			ui.RenderList(rows, showAll)
			return nil
		},
	}
	cmd.Flags().BoolVar(&showAll, "all", false, "Include unmanaged Git worktrees")
	cmd.Flags().BoolVar(&globalScope, "global", false, "List across all registered repositories")
	return cmd
}
