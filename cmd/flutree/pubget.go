package main

import (
	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraPub "github.com/EndersonPro/flutree/internal/infra/pub"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	"github.com/EndersonPro/flutree/internal/ui"
	"github.com/spf13/cobra"
)

func newPubGetCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "pubget <name>",
		Short: "Run pub get across workspace packages",
		Args:  requireName("Missing workspace name.", "Usage: flutree pubget <name> [--force]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			service := app.NewPubGetService(registry.NewDefault(), &infraPub.Gateway{})
			// The progress indicator writes to stdout, which must carry only the
			// JSON document in --json mode.
			stopLoading := func(bool) {}
			if !jsonOutput(cmd) {
				stopLoading = ui.StartLoading("Running pub get across workspace...")
			}
			result, err := service.Run(domain.PubGetInput{Name: args[0], Force: force})
			stopLoading(err == nil)
			if err != nil {
				return err
			}
			if jsonOutput(cmd) {
				printJSON(cmd.OutOrStdout(), result)
				return nil
			}
			ui.RenderPubGetSuccess(result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Clean cache and remove pubspec.lock before pub get")
	return cmd
}
