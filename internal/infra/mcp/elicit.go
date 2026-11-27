package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// This file holds the "ask the user" layer. When the client declared the
// elicitation capability in initialize, the server asks the user directly with
// a schema-backed form. Otherwise tools return a normal result with
// needs_user_input=true that tells the agent exactly what to ask; nothing is
// executed in either undecided case.

// elicitationTimeout bounds how long a tool waits for the user. Expiry is
// treated like a cancel. It is a variable so tests can shorten it.
var elicitationTimeout = 2 * time.Minute

// userQuestion is one question the agent must put to the user in the
// needs_user_input fallback.
type userQuestion struct {
	Field   string   `json:"field"`
	Prompt  string   `json:"prompt"`
	Type    string   `json:"type"`
	Options []string `json:"options,omitempty"`
}

type askOutcome int

const (
	askAccepted askOutcome = iota
	askDeclined
	askCancelled
	askUnsupported
)

// askResult is the user's answer to one elicitation.
type askResult struct {
	outcome askOutcome
	content map[string]any
}

// clientElicits reports whether the calling client declared form elicitation
// support in its initialize capabilities and the transport can carry the
// request. A context without a live session (direct handler calls) says no.
func clientElicits(ctx context.Context) bool {
	if mcpserver.ServerFromContext(ctx) == nil {
		return false
	}
	session := mcpserver.ClientSessionFromContext(ctx)
	if session == nil {
		return false
	}
	if _, ok := session.(mcpserver.SessionWithElicitation); !ok {
		return false
	}
	info, ok := session.(mcpserver.SessionWithClientInfo)
	if !ok {
		return false
	}
	caps := info.GetClientCapabilities().Elicitation
	if caps == nil {
		return false
	}
	// An empty capability object means form mode; only reject URL-only clients.
	return caps.Form != nil || caps.URL == nil
}

// askUser sends one form elicitation and waits for the user, up to
// elicitationTimeout. Timeouts and transport failures count as a cancel.
func askUser(ctx context.Context, message string, schema map[string]any) askResult {
	srv := mcpserver.ServerFromContext(ctx)
	if srv == nil {
		return askResult{outcome: askUnsupported}
	}
	askCtx, cancel := context.WithTimeout(ctx, elicitationTimeout)
	defer cancel()

	req := mcplib.ElicitationRequest{
		Request: mcplib.Request{Method: string(mcplib.MethodElicitationCreate)},
		Params: mcplib.ElicitationParams{
			Mode:            mcplib.ElicitationModeForm,
			Message:         message,
			RequestedSchema: schema,
		},
	}
	res, err := srv.RequestElicitation(askCtx, req)
	if err != nil {
		if errors.Is(err, mcpserver.ErrElicitationNotSupported) || errors.Is(err, mcpserver.ErrNoActiveSession) {
			return askResult{outcome: askUnsupported}
		}
		return askResult{outcome: askCancelled}
	}
	switch res.Action {
	case mcplib.ElicitationResponseActionAccept:
		content, _ := res.Content.(map[string]any)
		return askResult{outcome: askAccepted, content: content}
	case mcplib.ElicitationResponseActionDecline:
		return askResult{outcome: askDeclined}
	default:
		return askResult{outcome: askCancelled}
	}
}

// ── confirmation gate ────────────────────────────────────────────────────────

// confirmSpec describes one yes/no confirmation before a state-changing action.
type confirmSpec struct {
	// Message names exactly what the action will do (create or delete).
	Message string
	// Always asks the user even when the agent passed confirm=true, as long as
	// the client can elicit. Used for destructive actions.
	Always bool
	// Pending holds the result fields returned when nothing was executed
	// because confirmation is still needed (for example created=false).
	Pending map[string]any
	// Instruction tells the agent what to do in the fallback path.
	Instruction string
}

// requiresConfirmation is the single place that decides whether a
// state-changing tool must stop and ask before changing anything. It returns
// (nil, true) when the action may proceed; otherwise a ready tool result and
// false, and nothing may be executed.
//
// - Client with elicitation: the user is asked a yes/no question. confirm=true
// from the agent skips the question unless spec.Always is set. Accept with
// confirm true (or no explicit answer) proceeds; anything else returns a
// "declined" result.
// - Client without elicitation: confirm=true proceeds; otherwise the result has
// needs_user_input=true and the question for the agent to ask the user.
func requiresConfirmation(ctx context.Context, req mcplib.CallToolRequest, spec confirmSpec) (*mcpCallToolResult, bool) {
	flag := req.GetBool("confirm", false)

	if clientElicits(ctx) && (!flag || spec.Always) {
		ans := askUser(ctx, spec.Message, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"confirm": map[string]any{
					"type":        "boolean",
					"title":       "Confirm",
					"description": "Yes to proceed, no to leave everything unchanged.",
					"default":     false,
				},
			},
			"required": []string{"confirm"},
		})
		switch ans.outcome {
		case askAccepted:
			// Fail closed: only an explicit confirm=true proceeds. An accept
			// that omits the field (a non-conforming client) is a decline.
			if b, ok := ans.content["confirm"].(bool); !ok || !b {
				return declinedResult(spec.Pending, askDeclined), false
			}
			return nil, true
		case askDeclined, askCancelled:
			return declinedResult(spec.Pending, ans.outcome), false
		}
		// askUnsupported: fall through to the agent-driven path.
	}

	if flag {
		return nil, true
	}
	return needsInputResult(spec.Pending, spec.Instruction, []userQuestion{{
		Field:  "confirm",
		Prompt: spec.Message,
		Type:   "boolean",
	}}), false
}

// declinedResult is the tool result when the user said no or dismissed the
// question. It is a normal result, not an error.
func declinedResult(base map[string]any, outcome askOutcome) *mcpCallToolResult {
	status, what := "declined_by_user", "declined"
	if outcome == askCancelled {
		status, what = "cancelled_by_user", "cancelled (or did not answer in time)"
	}
	out := cloneMap(base)
	out["status"] = status
	out["user_declined"] = true
	out["message"] = "The user " + what + " the request. Nothing was executed. Do not retry on your own; ask the user what they want instead."
	res, _ := marshalStructured(out)
	return res
}

// needsInputResult is the fallback for clients without elicitation: a normal
// result telling the agent what to ask the user. Nothing was executed.
func needsInputResult(base map[string]any, instruction string, questions []userQuestion) *mcpCallToolResult {
	out := cloneMap(base)
	if _, ok := out["status"]; !ok {
		out["status"] = "user_input_required"
	}
	out["needs_user_input"] = true
	out["questions"] = questions
	out["instruction"] = instruction
	if _, ok := out["message"]; !ok {
		out["message"] = instruction
	}
	res, _ := marshalStructured(out)
	return res
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+4)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ── missing create inputs ────────────────────────────────────────────────────

// withArguments returns a copy of req whose arguments are replaced by args.
func withArguments(req mcplib.CallToolRequest, args map[string]any) mcplib.CallToolRequest {
	req.Params.Arguments = args
	return req
}

func copyArguments(req mcplib.CallToolRequest) map[string]any {
	out := map[string]any{}
	for k, v := range req.GetArguments() {
		out[k] = v
	}
	return out
}

// elicitCreateInputs asks the user for what plan_create/create_workspace lack
// when the client supports elicitation: the root base branch, the package list
// (only when the agent did not pass the argument at all) and the base branch of
// every package without one. Without elicitation it returns req unchanged, so
// buildCreateInput keeps reporting the missing input as an error with a hint.
// A non-nil result means the user declined or cancelled: stop and return it.
func elicitCreateInputs(ctx context.Context, svc MCPServices, req mcplib.CallToolRequest, pending map[string]any) (mcplib.CallToolRequest, *mcpCallToolResult) {
	if !clientElicits(ctx) {
		return req, nil
	}
	name := strings.TrimSpace(req.GetString("name", ""))
	rootRepo := strings.TrimSpace(req.GetString("root_repo", ""))
	if name == "" || rootRepo == "" {
		return req, nil // those are reported as input errors
	}

	args := copyArguments(req)
	scope := createScope(svc, req, rootRepo)

	baseBranch := strings.TrimSpace(req.GetString("base_branch", ""))
	_, packagesGiven := args["packages"]
	packages := req.GetStringSlice("packages", nil)

	// Form 1: root base branch and package list.
	props := map[string]any{}
	var required []string
	if baseBranch == "" {
		props["base_branch"] = branchSchema(svc, scope, rootRepo, "Base branch for the root worktree of '"+name+"'")
		required = append(required, "base_branch")
	}
	if !packagesGiven {
		if options := packageCandidates(svc, scope, rootRepo); len(options) > 0 {
			props["packages"] = map[string]any{
				"type":        "array",
				"title":       "Packages",
				"description": "Repositories to add as package worktrees. Leave empty for a root-only workspace.",
				"items":       map[string]any{"type": "string", "enum": options},
			}
		}
	}
	if len(props) > 0 {
		ans := askUser(ctx, "Workspace '"+name+"' needs a few decisions before it can be planned.", map[string]any{
			"type": "object", "properties": props, "required": required,
		})
		switch ans.outcome {
		case askDeclined, askCancelled:
			return req, declinedResult(pending, ans.outcome)
		case askUnsupported:
			return req, nil
		}
		if v, ok := ans.content["base_branch"].(string); ok && strings.TrimSpace(v) != "" {
			baseBranch = strings.TrimSpace(v)
			args["base_branch"] = baseBranch
		}
		if raw, ok := ans.content["packages"].([]any); ok {
			packages = packages[:0:0]
			for _, item := range raw {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					packages = append(packages, strings.TrimSpace(s))
				}
			}
			args["packages"] = packages
		}
	}

	// Form 2: base branch of every package that has none.
	packageBase := getStringMap(withArguments(req, args), "package_base")
	pkgProps := map[string]any{}
	var pkgRequired []string
	fieldToPackage := map[string]string{}
	for _, pkg := range packages {
		if hasPackageBase(packageBase, pkg) {
			continue
		}
		field := filepath.Base(filepath.Clean(pkg))
		fieldToPackage[field] = pkg
		pkgProps[field] = branchSchema(svc, scope, pkg, "Base branch for package '"+field+"'")
		pkgRequired = append(pkgRequired, field)
	}
	if len(pkgProps) > 0 {
		ans := askUser(ctx, "Which base branch should each package start from?", map[string]any{
			"type": "object", "properties": pkgProps, "required": pkgRequired,
		})
		switch ans.outcome {
		case askDeclined, askCancelled:
			return req, declinedResult(pending, ans.outcome)
		case askUnsupported:
			return req, nil
		}
		merged := map[string]any{}
		for k, v := range packageBase {
			merged[k] = v
		}
		for field, pkg := range fieldToPackage {
			if v, ok := ans.content[field].(string); ok && strings.TrimSpace(v) != "" {
				merged[filepath.Base(filepath.Clean(pkg))] = strings.TrimSpace(v)
			}
		}
		args["package_base"] = merged
	}
	return withArguments(req, args), nil
}

// createScope is the folder used to discover repositories for questions:
// the scope argument, the scope.root config, or the parent of a path root.
func createScope(svc MCPServices, req mcplib.CallToolRequest, rootRepo string) string {
	if scope := strings.TrimSpace(req.GetString("scope", "")); scope != "" {
		return scope
	}
	if svc.Config != nil {
		if configured, err := svc.Config.Get("scope.root"); err == nil && strings.TrimSpace(configured) != "" {
			return strings.TrimSpace(configured)
		}
	}
	if path := expandPath(rootRepo); path != "" && filepath.IsAbs(path) {
		return filepath.Dir(path)
	}
	return ""
}

func expandPath(sel string) string {
	sel = strings.TrimSpace(sel)
	if sel == "~" || strings.HasPrefix(sel, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			sel = filepath.Join(home, strings.TrimPrefix(sel, "~"))
		}
	}
	if !(strings.HasPrefix(sel, "/") || strings.HasPrefix(sel, ".")) && !strings.ContainsRune(sel, filepath.Separator) {
		return "" // a bare name, not a path
	}
	abs, err := filepath.Abs(sel)
	if err != nil {
		return ""
	}
	return abs
}

// resolveRepoPath maps a selector (path, folder name or package name) to a
// repository path, or "" when it cannot be resolved cheaply.
func resolveRepoPath(svc MCPServices, scope, selector string) string {
	if p := expandPath(selector); p != "" {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		return ""
	}
	if svc.Inspect == nil || scope == "" {
		return ""
	}
	repos, err := svc.Inspect.DiscoverRepos(scope)
	if err != nil {
		return ""
	}
	for _, r := range repos {
		if r.Name == selector || r.PackageName == selector || r.RepoRoot == selector {
			return r.RepoRoot
		}
	}
	return ""
}

// packageCandidates lists the names of repositories near the root that could
// be packages (every discovered repo except the root).
func packageCandidates(svc MCPServices, scope, rootRepo string) []string {
	if svc.Inspect == nil || scope == "" {
		return nil
	}
	repos, err := svc.Inspect.DiscoverRepos(scope)
	if err != nil {
		return nil
	}
	rootPath := resolveRepoPath(svc, scope, rootRepo)
	var out []string
	for _, r := range repos {
		if r.Name == rootRepo || r.PackageName == rootRepo || (rootPath != "" && r.RepoRoot == rootPath) {
			continue
		}
		out = append(out, r.Name)
	}
	return out
}

// branchSchema builds the string field for a base branch. When the repo's
// branches can be listed cheaply they become an enum (local first, then
// origin/*) and the current branch is the default.
func branchSchema(svc MCPServices, scope, selector, title string) map[string]any {
	field := map[string]any{"type": "string", "title": title}
	path := resolveRepoPath(svc, scope, selector)
	if path == "" || svc.Inspect == nil {
		field["description"] = "Branch (or tag/commit) to start from, for example main or develop."
		return field
	}
	branches, err := svc.Inspect.ListBranches(path)
	if err != nil {
		field["description"] = "Branch (or tag/commit) to start from, for example main or develop."
		return field
	}
	var options []string
	seen := map[string]bool{}
	for _, b := range append(append([]string{}, branches.Local...), branches.Remote...) {
		if b != "" && !seen[b] {
			seen[b] = true
			options = append(options, b)
		}
	}
	if len(options) == 0 {
		field["description"] = "Branch (or tag/commit) to start from, for example main or develop."
		return field
	}
	field["enum"] = options
	field["description"] = fmt.Sprintf("Branch to start from. Current branch: %s.", branches.Current)
	if seen[branches.Current] {
		field["default"] = branches.Current
	}
	return field
}
