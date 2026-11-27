package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
	"github.com/spf13/cobra"
)

// newMCPInstallCmd installs the flutree MCP server entry into each detected AI
// coding client's config file.
func newMCPInstallCmd() *cobra.Command {
	var clientFlag string
	var force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the flutree MCP server entry into detected AI coding clients",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var clients []string
			for _, c := range strings.Split(clientFlag, ",") {
				if t := strings.TrimSpace(c); t != "" {
					clients = append(clients, t)
				}
			}

			// Validate the client filter before any I/O so the error category is
			// correct: unknown-client is a user input error (exit 2); binary
			// resolution and other runtime failures are unexpected (exit 1).
			installer := mcpinstall.DefaultInstaller()
			if err := installer.ValidateClients(clients); err != nil {
				return domain.NewError(domain.CategoryInput, 2, err.Error(), "", err)
			}
			results, err := installer.Run(clients, force)
			if err != nil {
				return domain.NewError(domain.CategoryUnexpected, 1, err.Error(), "", err)
			}

			out := cmd.OutOrStdout()
			if jsonOutput(cmd) {
				byClient := make(map[string]mcpinstall.InstallResult, len(results))
				for _, r := range results {
					byClient[r.Client] = r
				}
				printJSON(out, byClient)
				return nil
			}

			hasError := false
			for _, r := range results {
				switch r.Status {
				case mcpinstall.OutcomeConfigured:
					fmt.Fprintf(out, "[OK]   %s: configured%s\n", r.Client, pathSuffix(r.ConfigPath))
				case mcpinstall.OutcomeAlreadyExists:
					fmt.Fprintf(out, "[SKIP] %s: already_exists%s\n", r.Client, pathSuffix(r.ConfigPath))
				case mcpinstall.OutcomeNotInstalled:
					fmt.Fprintf(out, "[-]    %s: not_installed\n", r.Client)
				case mcpinstall.OutcomeError:
					fmt.Fprintf(os.Stderr, "[ERR]  %s: %s\n", r.Client, r.Message)
					hasError = true
				}
			}
			if hasError {
				return domain.NewError(domain.CategoryUnexpected, 1, "One or more clients failed to install.", "", nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&clientFlag, "client", "", "Comma-separated list of clients to target (claude-code, opencode, codex)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing entries")
	return cmd
}

func pathSuffix(path string) string {
	if path == "" {
		return ""
	}
	return " (" + path + ")"
}
