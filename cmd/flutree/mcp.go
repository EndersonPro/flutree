package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraConfig "github.com/EndersonPro/flutree/internal/infra/config"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	infraMCP "github.com/EndersonPro/flutree/internal/infra/mcp"
	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
	"github.com/EndersonPro/flutree/internal/infra/prompt"
	infraPub "github.com/EndersonPro/flutree/internal/infra/pub"
	"github.com/EndersonPro/flutree/internal/infra/registry"
	infraSDK "github.com/EndersonPro/flutree/internal/infra/sdk"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run or install the MCP server for AI agents",
		Long:  "MCP (Model Context Protocol) integration commands.",
		Args:  noUnknownSubcommand,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newMCPServeCmd(), newMCPInstallCmd())
	return cmd
}

// newMCPServeCmd starts the MCP stdio server. It blocks until stdin is closed.
//
// --json is rejected wherever it appears ("flutree mcp serve --json" or
// "flutree --json mcp serve"): stdout is the JSON-RPC transport and JSON
// envelopes would corrupt the framing clients rely on.
func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP stdio server (for AI coding clients)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonOutput(cmd) {
				return domain.NewError(domain.CategoryInput, 2,
					"--json is not supported for 'mcp serve'.",
					"The stdout of 'mcp serve' is the JSON-RPC transport; --json output would corrupt it.",
					nil,
				)
			}
			configRepo := infraConfig.NewDefault()
			configService := app.NewConfigService(configRepo)
			services := infraMCP.MCPServices{
				List:     app.NewListService(&infraGit.Gateway{}, registry.NewDefault()),
				Create:   app.NewCreateService(&infraGit.Gateway{}, registry.NewDefault(), prompt.New()).WithSDK(&infraSDK.Gateway{}, configService.SDKManager),
				AddRepo:  app.NewAddRepoService(&infraGit.Gateway{}, registry.NewDefault(), prompt.New()).WithSDK(&infraSDK.Gateway{}, configService.SDKManager),
				Complete: app.NewCompleteService(&infraGit.Gateway{}, registry.NewDefault(), prompt.New()),
				PubGet:   app.NewPubGetService(registry.NewDefault(), &infraPub.Gateway{}),
				Clean:    app.NewCleanService(&infraGit.Gateway{}, registry.NewDefault(), &infraPub.Gateway{}),
				Config:   configService,
			}
			srv, err := infraMCP.BuildServer(displayVersion(), services)
			if err != nil {
				return domain.NewError(domain.CategoryUnexpected, 1, "Failed to build MCP server.", err.Error(), err)
			}
			return infraMCP.ServeStdio(srv)
		},
	}
}

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
