package main

import (
	"fmt"
	"strings"

	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraUpdate "github.com/EndersonPro/flutree/internal/infra/update"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var check, apply bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check or apply brew updates",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return domain.NewError(domain.CategoryInput, 2, "Update command does not accept positional arguments.", "Usage: flutree update [--check|--apply]", nil)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			service := app.NewUpdateService(infraUpdate.NewBrewGateway())
			result, err := service.Run(domain.UpdateInput{Check: check, Apply: apply})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if jsonOutput(cmd) {
				printJSON(out, result)
				return nil
			}
			fmt.Fprintf(out, "mode=%s outdated=%t current=%s latest=%s\n", result.Mode, result.Outdated, safeVersion(result.Current), safeVersion(result.Latest))
			if result.Mode != "check" && strings.TrimSpace(result.UpgradeNotes) != "" {
				fmt.Fprintln(out, result.UpgradeNotes)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Check whether a brew update is available")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply brew update now")
	return cmd
}
