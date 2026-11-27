package main

import (
	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
	"github.com/spf13/cobra"
)

// newMCPUninstallCmd removes only the flutree MCP entry from detected clients.
func newMCPUninstallCmd() *cobra.Command {
	var clientFlag, scope string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the flutree MCP server entry from AI coding clients",
		Long: "Remove only the flutree entry from each detected client; every other key and server is preserved.\n\n" +
			"Outcomes: removed, not_found (no flutree entry), not_installed (client not detected), unsupported_scope.\n" +
			"Clients: " + clientsHelp + ".",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			installer := mcpinstall.DefaultInstaller()
			opts, err := mcpOptions(installer, clientFlag, scope, dryRun)
			if err != nil {
				return err
			}
			results, err := installer.Uninstall(opts)
			if err != nil {
				return domain.NewError(domain.CategoryUnexpected, 1, err.Error(), "", err)
			}
			if reportResults(cmd, results) {
				return domain.NewError(domain.CategoryUnexpected, 1, "One or more clients failed to uninstall.", "", nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&clientFlag, "client", "", "Comma-separated list of clients to target ("+clientsHelp+")")
	cmd.Flags().StringVar(&scope, "scope", "user", "Config scope: user (default) or project (current directory)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be removed without writing")
	return cmd
}
