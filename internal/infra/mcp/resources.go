package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/EndersonPro/flutree/internal/domain"
)

const (
	guideURI             = "flutree://guide"
	workspacesURI        = "flutree://workspaces"
	configURI            = "flutree://config"
	workspaceURIPrefix   = "flutree://workspace/"
	workspaceURITemplate = workspaceURIPrefix + "{name}"
)

// configKeys are the keys exposed by flutree://config.
var configKeys = []string{"scope.root", "workspace.generate", "sdk.manager"}

// registerResources adds the read-only resources agents use to learn the model
// and to look at the current state without calling a tool.
func registerResources(s *mcpserver.MCPServer, svc MCPServices) {
	s.AddResource(
		mcplib.NewResource(guideURI, "flutree guide",
			mcplib.WithResourceDescription("How flutree workspaces work (layout, branches, pubspec_overrides.yaml, registry, SDK setup, config) and the recommended agent flow, with examples. Read it once before planning a workspace."),
			mcplib.WithMIMEType("text/markdown")),
		func(_ context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			return textResource(req.Params.URI, "text/markdown", guideMarkdown), nil
		},
	)

	s.AddResource(
		mcplib.NewResource(workspacesURI, "flutree workspaces",
			mcplib.WithResourceDescription("JSON list of every workspace flutree manages (same rows as the list_worktrees tool with global_scope=true): name, branch, path, repo_root, status, package_count."),
			mcplib.WithMIMEType("application/json")),
		func(_ context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			rows, err := svc.List.Run(domain.ListInput{GlobalScope: true})
			if err != nil {
				return nil, resourceError(err)
			}
			if rows == nil {
				rows = []domain.ListRow{}
			}
			return jsonResource(req.Params.URI, rows)
		},
	)

	s.AddResourceTemplate(
		mcplib.NewResourceTemplate(workspaceURITemplate, "flutree workspace",
			mcplib.WithTemplateDescription("State of one workspace by name: branch, root and package worktrees (name, path, repo_root, branch, exists, dirty), base_branch with ahead/behind counts when known (else ahead_behind_note), the parsed pubspec_overrides.yaml entries and the .code-workspace file path. Same data as the workspace_status tool."),
			mcplib.WithTemplateMIMEType("application/json")),
		func(_ context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			name, err := url.PathUnescape(strings.TrimPrefix(req.Params.URI, workspaceURIPrefix))
			if err != nil || strings.TrimSpace(name) == "" {
				return nil, fmt.Errorf("invalid workspace URI %q: expected %s", req.Params.URI, workspaceURITemplate)
			}
			status, err := svc.Inspect.WorkspaceStatus(name)
			if err != nil {
				return nil, resourceError(err)
			}
			return jsonResource(req.Params.URI, status)
		},
	)

	s.AddResource(
		mcplib.NewResource(configURI, "flutree config",
			mcplib.WithResourceDescription("Effective flutree configuration as JSON: scope.root (empty when unset), workspace.generate (true by default) and sdk.manager (auto by default)."),
			mcplib.WithMIMEType("application/json")),
		func(_ context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			values := make(map[string]string, len(configKeys))
			for _, key := range configKeys {
				value, err := svc.Config.Get(key)
				if err != nil {
					return nil, resourceError(err)
				}
				values[key] = value
			}
			return jsonResource(req.Params.URI, values)
		},
	)
}

func textResource(uri, mime, text string) []mcplib.ResourceContents {
	return []mcplib.ResourceContents{mcplib.TextResourceContents{URI: uri, MIMEType: mime, Text: text}}
}

func jsonResource(uri string, v any) ([]mcplib.ResourceContents, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", uri, err)
	}
	return textResource(uri, "application/json", string(raw)), nil
}

// resourceError flattens an app error (message plus hint) into a plain error,
// which mcp-go reports as a JSON-RPC error: resources have no isError result.
func resourceError(err error) error {
	if appErr, ok := err.(*domain.AppError); ok && appErr.Hint != "" {
		return fmt.Errorf("%s (%s)", appErr.Message, appErr.Hint)
	}
	return err
}
