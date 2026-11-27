package main

import (
	"fmt"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version",
		Args:  validateVersionArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printVersion(cmd)
		},
	}
}

func validateVersionArgs(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return domain.NewError(domain.CategoryInput, 2, "Version command does not accept arguments.", "Use 'flutree version' or 'flutree --version'.", nil)
	}
	return nil
}

func printVersion(cmd *cobra.Command) error {
	if jsonOutput(cmd) {
		printJSON(cmd.OutOrStdout(), map[string]string{"version": displayVersion()})
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), displayVersion())
	return nil
}
