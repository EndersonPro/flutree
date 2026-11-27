package main

import (
	"fmt"

	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraConfig "github.com/EndersonPro/flutree/internal/infra/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage persisted CLI configuration",
		Long: `Manage persisted CLI configuration values.

Supported keys:
  scope.root           Default discovery root for create/add-repo when --scope is omitted
  workspace.generate   Whether create writes a VSCode .code-workspace when neither
                       --workspace nor --no-workspace is passed (true|false, default true)`,
		Example: `  flutree config set scope.root <path>
  flutree config get scope.root
  flutree config set workspace.generate false`,
		Args: noUnknownSubcommand,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigSetCmd(), newConfigGetCmd())
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  exactConfigArgs(2, "Invalid config set arguments.", "Usage: flutree config set <key> <value>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			stored, err := app.NewConfigService(infraConfig.NewDefault()).Set(args[0], args[1])
			if err != nil {
				return err
			}
			return printConfigValue(cmd, args[0], stored)
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print a configuration value",
		Args:  exactConfigArgs(1, "Invalid config get arguments.", "Usage: flutree config get <key>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := app.NewConfigService(infraConfig.NewDefault()).Get(args[0])
			if err != nil {
				return err
			}
			return printConfigValue(cmd, args[0], value)
		},
	}
}

func exactConfigArgs(n int, message, hint string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != n {
			return domain.NewError(domain.CategoryInput, 2, message, hint, nil)
		}
		return nil
	}
}

func printConfigValue(cmd *cobra.Command, key, value string) error {
	if jsonOutput(cmd) {
		printJSON(cmd.OutOrStdout(), map[string]string{"key": key, "value": value})
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), value)
	return nil
}
