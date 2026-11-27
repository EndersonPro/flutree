package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
	"github.com/spf13/cobra"
)

const clientsHelp = "claude-code, opencode, codex, cursor, vscode, gemini, claude-desktop, windsurf"

// mcpOptions turns the shared --client/--scope/--dry-run flags into Options,
// validating before any I/O so bad input exits 2.
func mcpOptions(installer *mcpinstall.Installer, clientFlag, scope string, dryRun bool) (mcpinstall.Options, error) {
	var clients []string
	for _, c := range strings.Split(clientFlag, ",") {
		if t := strings.TrimSpace(c); t != "" {
			clients = append(clients, t)
		}
	}
	if err := installer.ValidateClients(clients); err != nil {
		return mcpinstall.Options{}, domain.NewError(domain.CategoryInput, 2, err.Error(), "", err)
	}
	if scope != string(mcpinstall.ScopeUser) && scope != string(mcpinstall.ScopeProject) {
		msg := fmt.Sprintf("invalid --scope %q; valid values: user, project", scope)
		return mcpinstall.Options{}, domain.NewError(domain.CategoryInput, 2, msg, "", nil)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return mcpinstall.Options{}, domain.NewError(domain.CategoryUnexpected, 1, err.Error(), "", err)
	}
	return mcpinstall.Options{Clients: clients, DryRun: dryRun, Scope: mcpinstall.Scope(scope), Cwd: cwd}, nil
}

// reportResults prints per-client results (JSON keyed by client, or text lines)
// and reports whether any client failed.
func reportResults(cmd *cobra.Command, results []mcpinstall.InstallResult) bool {
	out := cmd.OutOrStdout()
	if jsonOutput(cmd) {
		byClient := make(map[string]mcpinstall.InstallResult, len(results))
		for _, r := range results {
			byClient[r.Client] = r
		}
		printJSON(out, byClient)
		for _, r := range results {
			if r.Status == mcpinstall.OutcomeError {
				return true
			}
		}
		return false
	}
	hasError := false
	for _, r := range results {
		prefix := ""
		if r.DryRun {
			prefix = "would be "
		}
		switch r.Status {
		case mcpinstall.OutcomeConfigured:
			fmt.Fprintf(out, "[OK]   %s: %sconfigured%s\n", r.Client, prefix, pathSuffix(r.ConfigPath))
			printEntry(out, r)
		case mcpinstall.OutcomeRemoved:
			fmt.Fprintf(out, "[OK]   %s: %sremoved%s\n", r.Client, prefix, pathSuffix(r.ConfigPath))
		case mcpinstall.OutcomeAlreadyExists:
			fmt.Fprintf(out, "[SKIP] %s: already_exists%s\n", r.Client, pathSuffix(r.ConfigPath))
		case mcpinstall.OutcomeNotFound:
			fmt.Fprintf(out, "[SKIP] %s: not_found%s\n", r.Client, pathSuffix(r.ConfigPath))
		case mcpinstall.OutcomeNotInstalled:
			fmt.Fprintf(out, "[-]    %s: not_installed\n", r.Client)
		case mcpinstall.OutcomeUnsupportedScope:
			fmt.Fprintf(out, "[-]    %s: unsupported_scope (%s)\n", r.Client, r.Message)
		case mcpinstall.OutcomeError:
			fmt.Fprintf(os.Stderr, "[ERR]  %s: %s\n", r.Client, r.Message)
			hasError = true
		}
	}
	return hasError
}

func printEntry(out io.Writer, r mcpinstall.InstallResult) {
	if r.Entry == "" {
		return
	}
	for _, line := range strings.Split(r.Entry, "\n") {
		fmt.Fprintf(out, "         %s\n", line)
	}
}

// newMCPInstallCmd installs the flutree MCP server entry into each detected AI
// coding client's config file.
func newMCPInstallCmd() *cobra.Command {
	var clientFlag, scope string
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the flutree MCP server entry into detected AI coding clients",
		Long: "Install the flutree MCP server entry into every detected AI coding client.\n\n" +
			"Clients: " + clientsHelp + ".\n" +
			"Existing keys and other servers are preserved; an existing flutree entry is kept unless --force.\n" +
			"Use --scope project to write the project config of the current directory " +
			"(clients without one report unsupported_scope) and --dry-run to preview without writing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			installer := mcpinstall.DefaultInstaller()
			opts, err := mcpOptions(installer, clientFlag, scope, dryRun)
			if err != nil {
				return err
			}
			opts.Force = force
			results, err := installer.Install(opts)
			if err != nil {
				return domain.NewError(domain.CategoryUnexpected, 1, err.Error(), "", err)
			}
			if reportResults(cmd, results) {
				return domain.NewError(domain.CategoryUnexpected, 1, "One or more clients failed to install.", "", nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&clientFlag, "client", "", "Comma-separated list of clients to target ("+clientsHelp+")")
	cmd.Flags().StringVar(&scope, "scope", "user", "Config scope: user (default) or project (current directory)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing entries")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show the target file and entry per client without writing")
	return cmd
}

func pathSuffix(path string) string {
	if path == "" {
		return ""
	}
	return " (" + path + ")"
}
