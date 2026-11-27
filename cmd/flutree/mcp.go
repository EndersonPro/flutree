package main

import (
	"github.com/EndersonPro/flutree/internal/app"
	"github.com/EndersonPro/flutree/internal/domain"
	infraConfig "github.com/EndersonPro/flutree/internal/infra/config"
	infraGit "github.com/EndersonPro/flutree/internal/infra/git"
	infraMCP "github.com/EndersonPro/flutree/internal/infra/mcp"
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
	cmd.AddCommand(newMCPServeCmd(), newMCPInstallCmd(), newMCPUninstallCmd(), newMCPDoctorCmd())
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
				Inspect:  app.NewInspectService(&infraGit.Gateway{}, &infraSDK.Gateway{}, registry.NewDefault()),
			}
			srv, err := infraMCP.BuildServer(displayVersion(), services)
			if err != nil {
				return domain.NewError(domain.CategoryUnexpected, 1, "Failed to build MCP server.", err.Error(), err)
			}
			return infraMCP.ServeStdio(srv)
		},
	}
}
