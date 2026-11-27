package mcpinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// jsonSpec describes a client whose MCP servers live in a JSON object.
type jsonSpec struct {
	name   string
	binary string // executable looked up on PATH for detection; "" disables
	key    string // object holding the servers
	// userPath and projectPath return the config file; projectPath is nil when
	// the client has no project scope.
	userPath    func(home string) string
	projectPath func(cwd string) string
	// detectDirs returns directories whose existence proves the client is installed.
	detectDirs func(home string) []string
	entry      func(absCmd string) map[string]any
}

type jsonClient struct {
	spec     jsonSpec
	baseDir  string
	lookPath func(string) (string, error)
	scope    Scope
	cwd      string
}

func newJSONClient(spec jsonSpec, baseDir string) Client {
	lp := exec.LookPath
	if baseDir != "" {
		lp = func(string) (string, error) { return "", fmt.Errorf("not found") }
	}
	return &jsonClient{spec: spec, baseDir: baseDir, lookPath: lp, scope: ScopeUser}
}

func stdioEntry(withType bool) func(string) map[string]any {
	return func(absCmd string) map[string]any {
		e := map[string]any{"command": absCmd, "args": []string{"mcp", "serve"}}
		if withType {
			e["type"] = "stdio"
		}
		return e
	}
}

// appSupportDir returns the per-OS application data directory of an Electron
// style app, e.g. "Claude" -> ~/.config/Claude on Linux.
func appSupportDir(home, app string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", app)
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, app)
	default:
		return filepath.Join(home, ".config", app)
	}
}

// NewCursorMerger returns the Cursor client.
// Source: https://cursor.com/docs/context/mcp (global ~/.cursor/mcp.json,
// project .cursor/mcp.json, top-level "mcpServers", stdio entry with type/command/args).
func NewCursorMerger(baseDir string) Client {
	return newJSONClient(jsonSpec{
		name: "cursor", binary: "cursor", key: "mcpServers",
		userPath:    func(h string) string { return filepath.Join(h, ".cursor", "mcp.json") },
		projectPath: func(c string) string { return filepath.Join(c, ".cursor", "mcp.json") },
		detectDirs:  func(h string) []string { return []string{filepath.Join(h, ".cursor")} },
		entry:       stdioEntry(true),
	}, baseDir)
}

// NewVSCodeMerger returns the VS Code (GitHub Copilot) client.
// Source: https://code.visualstudio.com/docs/copilot/reference/mcp-configuration
// (workspace .vscode/mcp.json and profile mcp.json, top-level "servers", stdio
// entry with type/command/args). The per-OS profile directory (Code/User) is
// not spelled out on that page; it is the standard VS Code user-data location.
func NewVSCodeMerger(baseDir string) Client {
	return newJSONClient(jsonSpec{
		name: "vscode", binary: "code", key: "servers",
		userPath: func(h string) string {
			return filepath.Join(appSupportDir(h, "Code"), "User", "mcp.json")
		},
		projectPath: func(c string) string { return filepath.Join(c, ".vscode", "mcp.json") },
		detectDirs:  func(h string) []string { return []string{appSupportDir(h, "Code")} },
		entry:       stdioEntry(true),
	}, baseDir)
}

// NewGeminiMerger returns the Gemini CLI client.
// Source: https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md
// (user ~/.gemini/settings.json, project .gemini/settings.json, "mcpServers",
// stdio entry with command/args).
func NewGeminiMerger(baseDir string) Client {
	return newJSONClient(jsonSpec{
		name: "gemini", binary: "gemini", key: "mcpServers",
		userPath:    func(h string) string { return filepath.Join(h, ".gemini", "settings.json") },
		projectPath: func(c string) string { return filepath.Join(c, ".gemini", "settings.json") },
		detectDirs:  func(h string) []string { return []string{filepath.Join(h, ".gemini")} },
		entry:       stdioEntry(false),
	}, baseDir)
}

// NewClaudeDesktopMerger returns the Claude Desktop client (user scope only).
// Source: https://modelcontextprotocol.io/quickstart/user (macOS
// ~/Library/Application Support/Claude/, Windows %APPDATA%\Claude\, Linux
// ~/.config/Claude/ claude_desktop_config.json, "mcpServers").
func NewClaudeDesktopMerger(baseDir string) Client {
	return newJSONClient(jsonSpec{
		name: "claude-desktop", key: "mcpServers",
		userPath: func(h string) string {
			return filepath.Join(appSupportDir(h, "Claude"), "claude_desktop_config.json")
		},
		detectDirs: func(h string) []string { return []string{appSupportDir(h, "Claude")} },
		entry:      stdioEntry(false),
	}, baseDir)
}

// NewWindsurfMerger returns the Windsurf client (user scope only).
// UNVERIFIED against official docs: docs.windsurf.com now redirects to the
// Devin docs, which no longer describe ~/.codeium/windsurf/mcp_config.json.
// The path and "mcpServers" shape follow Windsurf's long-standing convention.
func NewWindsurfMerger(baseDir string) Client {
	return newJSONClient(jsonSpec{
		name: "windsurf", binary: "windsurf", key: "mcpServers",
		userPath:   func(h string) string { return filepath.Join(h, ".codeium", "windsurf", "mcp_config.json") },
		detectDirs: func(h string) []string { return []string{filepath.Join(h, ".codeium", "windsurf")} },
		entry:      stdioEntry(false),
	}, baseDir)
}

func (j *jsonClient) Name() string { return j.spec.name }

func (j *jsonClient) WithScope(scope Scope, cwd string) (Client, bool) {
	if scope == ScopeProject && j.spec.projectPath == nil {
		return nil, false
	}
	cp := *j
	cp.scope, cp.cwd = scope, cwd
	return &cp, true
}

func (j *jsonClient) ConfigPath() string {
	if j.scope == ScopeProject {
		return j.spec.projectPath(j.cwd)
	}
	home, err := homeOf(j.baseDir)
	if err != nil {
		return ""
	}
	return j.spec.userPath(home)
}

func (j *jsonClient) Detect() (bool, error) {
	if j.spec.binary != "" {
		if _, err := j.lookPath(j.spec.binary); err == nil {
			return true, nil
		}
	}
	home, err := homeOf(j.baseDir)
	if err != nil {
		return false, fmt.Errorf("%s: %w", j.spec.name, err)
	}
	for _, d := range j.spec.detectDirs(home) {
		if dirExists(d) {
			return true, nil
		}
	}
	return false, nil
}

func (j *jsonClient) Merge(absCmd string, force bool) (Outcome, error) {
	path := j.ConfigPath()
	if path == "" {
		return OutcomeError, fmt.Errorf("%s: cannot resolve config path", j.spec.name)
	}
	root, err := loadJSONMap(path)
	if err != nil {
		return OutcomeError, fmt.Errorf("%s: read config: %w", j.spec.name, err)
	}
	servers, err := getOrCreateRawMap(root, j.spec.key)
	if err != nil {
		return OutcomeError, fmt.Errorf("%s: %s key: %w", j.spec.name, j.spec.key, err)
	}
	if _, exists := servers[entryName]; exists && !force {
		return OutcomeAlreadyExists, nil
	}
	raw, err := json.Marshal(j.spec.entry(absCmd))
	if err != nil {
		return OutcomeError, fmt.Errorf("%s: marshal entry: %w", j.spec.name, err)
	}
	servers[entryName] = raw
	ser, err := json.Marshal(servers)
	if err != nil {
		return OutcomeError, fmt.Errorf("%s: marshal %s: %w", j.spec.name, j.spec.key, err)
	}
	root[j.spec.key] = ser
	if err := writeJSONMap(path, root); err != nil {
		return OutcomeError, fmt.Errorf("%s: write config: %w", j.spec.name, err)
	}
	return OutcomeConfigured, nil
}

func (j *jsonClient) Preview(absCmd string) (string, error) {
	return previewJSON(j.spec.entry(absCmd))
}

func (j *jsonClient) Remove() (Outcome, error) {
	out, err := jsonRemoveEntry(j.ConfigPath(), j.spec.key)
	if err != nil {
		return OutcomeError, fmt.Errorf("%s: %w", j.spec.name, err)
	}
	return out, nil
}

func (j *jsonClient) EntryCommand() (string, bool, error) {
	return jsonLookupCommand(j.ConfigPath(), j.spec.key)
}
