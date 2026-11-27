//go:build !windows

package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installEnv returns an environment with an empty PATH dir so client detection
// depends only on the marker dirs the test creates under HOME.
func installEnv(t *testing.T, home string) []string {
	t.Helper()
	return testEnvWithPath(home, t.TempDir())
}

func appDir(home, app string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", app)
	}
	return filepath.Join(home, ".config", app)
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(p))
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s is not valid JSON: %v\n%s", p, err, b)
	}
	return m
}

type jsonClientCase struct {
	name    string
	marker  func(home string) string // directory created to simulate detection
	file    func(home string) string // user-scope config file
	key     string
	project string // project-scope file relative to cwd; "" = unsupported
	typed   bool   // entry carries "type":"stdio"
}

func jsonClientCases() []jsonClientCase {
	return []jsonClientCase{
		{"cursor", func(h string) string { return filepath.Join(h, ".cursor") },
			func(h string) string { return filepath.Join(h, ".cursor", "mcp.json") }, "mcpServers", ".cursor/mcp.json", true},
		{"vscode", func(h string) string { return appDir(h, "Code") },
			func(h string) string { return filepath.Join(appDir(h, "Code"), "User", "mcp.json") }, "servers", ".vscode/mcp.json", true},
		{"gemini", func(h string) string { return filepath.Join(h, ".gemini") },
			func(h string) string { return filepath.Join(h, ".gemini", "settings.json") }, "mcpServers", ".gemini/settings.json", false},
		{"claude-desktop", func(h string) string { return appDir(h, "Claude") },
			func(h string) string { return filepath.Join(appDir(h, "Claude"), "claude_desktop_config.json") }, "mcpServers", "", false},
		{"windsurf", func(h string) string { return filepath.Join(h, ".codeium", "windsurf") },
			func(h string) string { return filepath.Join(h, ".codeium", "windsurf", "mcp_config.json") }, "mcpServers", "", false},
	}
}

func assertEntry(t *testing.T, cfg map[string]any, key string, typed bool) {
	t.Helper()
	servers, ok := cfg[key].(map[string]any)
	if !ok {
		t.Fatalf("expected %q object, got %v", key, cfg)
	}
	e, ok := servers["flutree"].(map[string]any)
	if !ok {
		t.Fatalf("expected %s.flutree object, got %v", key, servers)
	}
	if cmd, _ := e["command"].(string); !filepath.IsAbs(cmd) {
		t.Fatalf("command must be absolute, got %v", e["command"])
	}
	args, _ := e["args"].([]any)
	if len(args) != 2 || args[0] != "mcp" || args[1] != "serve" {
		t.Fatalf("args = %v, want [mcp serve]", e["args"])
	}
	if typed && e["type"] != "stdio" {
		t.Fatalf("type = %v, want stdio", e["type"])
	}
}

func TestMCPInstallNewClientsCreateEntryAndPreserveExisting(t *testing.T) {
	for _, tc := range jsonClientCases() {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			mustMkdir(t, tc.marker(home))
			mustWrite(t, tc.file(home), `{"theme":"dark","`+tc.key+`":{"other":{"command":"x"}}}`)

			res := runE2E(t, t.TempDir(), installEnv(t, home), "mcp", "install", "--client", tc.name)
			res.requireCode(t, 0)

			cfg := readJSON(t, tc.file(home))
			if cfg["theme"] != "dark" {
				t.Fatalf("unrelated key lost: %v", cfg)
			}
			assertEntry(t, cfg, tc.key, tc.typed)
			if _, ok := cfg[tc.key].(map[string]any)["other"]; !ok {
				t.Fatalf("existing server lost: %v", cfg)
			}

			// Second run keeps the entry; --force overwrites.
			res = runE2E(t, t.TempDir(), installEnv(t, home), "mcp", "install", "--client", tc.name)
			res.requireCode(t, 0)
			if !strings.Contains(res.stdout, "already_exists") {
				t.Fatalf("expected already_exists, got %s", res.stdout)
			}
			res = runE2E(t, t.TempDir(), installEnv(t, home), "mcp", "install", "--client", tc.name, "--force")
			res.requireCode(t, 0)
			if !strings.Contains(res.stdout, "configured") {
				t.Fatalf("expected configured with --force, got %s", res.stdout)
			}
		})
	}
}

func TestMCPInstallNewClientsNotInstalledWhenNotDetected(t *testing.T) {
	home := t.TempDir()
	res := runE2E(t, t.TempDir(), installEnv(t, home), "mcp", "install", "--json")
	res.requireCode(t, 0)
	var got map[string]struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, res.stdout)
	}
	for _, c := range []string{"claude-code", "opencode", "codex", "cursor", "vscode", "gemini", "claude-desktop", "windsurf"} {
		if got[c].Status != "not_installed" {
			t.Fatalf("%s status = %q, want not_installed (all: %s)", c, got[c].Status, res.stdout)
		}
	}
}

func TestMCPInstallScopeProject(t *testing.T) {
	for _, tc := range jsonClientCases() {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			cwd := t.TempDir()
			mustMkdir(t, tc.marker(home))

			res := runE2E(t, cwd, installEnv(t, home), "mcp", "install", "--client", tc.name, "--scope", "project", "--json")
			res.requireCode(t, 0)
			var got map[string]struct {
				Status     string `json:"status"`
				ConfigPath string `json:"config_path"`
			}
			if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
				t.Fatalf("bad JSON: %v\n%s", err, res.stdout)
			}
			if tc.project == "" {
				if got[tc.name].Status != "unsupported_scope" {
					t.Fatalf("status = %q, want unsupported_scope", got[tc.name].Status)
				}
				if _, err := os.Stat(tc.file(home)); err == nil {
					t.Fatal("unsupported scope must not write the user config")
				}
				return
			}
			assertEntry(t, readJSON(t, filepath.Join(cwd, tc.project)), tc.key, tc.typed)
			if _, err := os.Stat(tc.file(home)); err == nil {
				t.Fatal("project scope must not write the user config")
			}
		})
	}
}

func TestMCPInstallScopeProjectExistingClients(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	mustMkdir(t, filepath.Join(home, ".claude"))
	mustMkdir(t, filepath.Join(home, ".config", "opencode"))
	mustMkdir(t, filepath.Join(home, ".codex"))

	res := runE2E(t, cwd, installEnv(t, home), "mcp", "install", "--scope", "project",
		"--client", "claude-code,opencode,codex")
	res.requireCode(t, 0)
	assertEntry(t, readJSON(t, filepath.Join(cwd, ".mcp.json")), "mcpServers", true)
	oc := readJSON(t, filepath.Join(cwd, "opencode.json"))
	if _, ok := oc["mcp"].(map[string]any)["servers"].(map[string]any)["flutree"]; !ok {
		t.Fatalf("opencode project entry missing: %v", oc)
	}
	toml, err := os.ReadFile(filepath.Join(cwd, ".codex", "config.toml"))
	if err != nil || !strings.Contains(string(toml), "mcp_servers.flutree") {
		t.Fatalf("codex project entry missing: %v %s", err, toml)
	}
}

func TestMCPInstallInvalidScopeExits2(t *testing.T) {
	res := runE2E(t, t.TempDir(), installEnv(t, t.TempDir()), "mcp", "install", "--scope", "galaxy")
	res.requireCode(t, 2)
}

func TestMCPInstallDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	for _, tc := range jsonClientCases() {
		mustMkdir(t, tc.marker(home))
	}

	res := runE2E(t, cwd, installEnv(t, home), "mcp", "install", "--dry-run")
	res.requireCode(t, 0)
	if !strings.Contains(res.stdout, `"command"`) || !strings.Contains(res.stdout, "mcp") {
		t.Fatalf("dry-run must print the entry, got:\n%s", res.stdout)
	}
	for _, tc := range jsonClientCases() {
		if !strings.Contains(res.stdout, tc.file(home)) {
			t.Fatalf("dry-run must print %s target %s, got:\n%s", tc.name, tc.file(home), res.stdout)
		}
		if _, err := os.Stat(tc.file(home)); err == nil {
			t.Fatalf("dry-run wrote %s", tc.file(home))
		}
	}

	res = runE2E(t, cwd, installEnv(t, home), "mcp", "install", "--dry-run", "--json", "--client", "cursor")
	res.requireCode(t, 0)
	var got map[string]struct {
		Status     string `json:"status"`
		ConfigPath string `json:"config_path"`
		Entry      string `json:"entry"`
		DryRun     bool   `json:"dry_run"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, res.stdout)
	}
	c := got["cursor"]
	if !c.DryRun || c.Status != "configured" || c.ConfigPath != jsonClientCases()[0].file(home) || !strings.Contains(c.Entry, `"mcp"`) {
		t.Fatalf("unexpected dry-run JSON: %s", res.stdout)
	}
	if _, err := os.Stat(c.ConfigPath); err == nil {
		t.Fatal("dry-run --json wrote the config")
	}
}

func TestMCPUninstallRemovesOnlyFlutree(t *testing.T) {
	for _, tc := range jsonClientCases() {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			mustMkdir(t, tc.marker(home))
			mustWrite(t, tc.file(home), `{"theme":"dark","`+tc.key+`":{"other":{"command":"x"}}}`)
			env := installEnv(t, home)
			runE2E(t, t.TempDir(), env, "mcp", "install", "--client", tc.name).requireCode(t, 0)

			// dry-run keeps the entry.
			res := runE2E(t, t.TempDir(), env, "mcp", "uninstall", "--client", tc.name, "--dry-run")
			res.requireCode(t, 0)
			assertEntry(t, readJSON(t, tc.file(home)), tc.key, tc.typed)

			res = runE2E(t, t.TempDir(), env, "mcp", "uninstall", "--client", tc.name)
			res.requireCode(t, 0)
			if !strings.Contains(res.stdout, "removed") {
				t.Fatalf("expected removed, got %s", res.stdout)
			}
			cfg := readJSON(t, tc.file(home))
			servers := cfg[tc.key].(map[string]any)
			if _, ok := servers["flutree"]; ok {
				t.Fatalf("flutree not removed: %v", cfg)
			}
			if _, ok := servers["other"]; !ok || cfg["theme"] != "dark" {
				t.Fatalf("uninstall removed more than flutree: %v", cfg)
			}

			res = runE2E(t, t.TempDir(), env, "mcp", "uninstall", "--client", tc.name, "--json")
			res.requireCode(t, 0)
			if !strings.Contains(res.stdout, `"not_found"`) {
				t.Fatalf("expected not_found, got %s", res.stdout)
			}
		})
	}
}

func TestMCPUninstallNotInstalled(t *testing.T) {
	res := runE2E(t, t.TempDir(), installEnv(t, t.TempDir()), "mcp", "uninstall", "--client", "cursor", "--json")
	res.requireCode(t, 0)
	if !strings.Contains(res.stdout, `"not_installed"`) {
		t.Fatalf("expected not_installed, got %s", res.stdout)
	}
}

func TestMCPUninstallCodexKeepsOtherServers(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".codex", "config.toml"), "model = \"o3\"\n[mcp_servers.other]\ncommand = \"x\"\n")
	env := installEnv(t, home)
	runE2E(t, t.TempDir(), env, "mcp", "install", "--client", "codex").requireCode(t, 0)
	runE2E(t, t.TempDir(), env, "mcp", "uninstall", "--client", "codex").requireCode(t, 0)
	b, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	s := string(b)
	if strings.Contains(s, "flutree") || !strings.Contains(s, "other") || !strings.Contains(s, "o3") {
		t.Fatalf("unexpected codex config after uninstall:\n%s", s)
	}
}

type doctorJSON struct {
	OK        bool `json:"ok"`
	Handshake struct {
		OK         bool   `json:"ok"`
		ServerName string `json:"server_name"`
	} `json:"handshake"`
	Clients []struct {
		Client       string `json:"client"`
		Detected     bool   `json:"detected"`
		ConfigPath   string `json:"config_path"`
		EntryPresent bool   `json:"entry_present"`
		CommandOK    bool   `json:"command_ok"`
	} `json:"clients"`
}

func runDoctor(t *testing.T, home string) (e2eResult, doctorJSON) {
	t.Helper()
	res := runE2E(t, t.TempDir(), installEnv(t, home), "mcp", "doctor", "--json")
	var d doctorJSON
	if err := json.Unmarshal([]byte(res.stdout), &d); err != nil {
		t.Fatalf("bad doctor JSON: %v\nstdout=%s\nstderr=%s", err, res.stdout, res.stderr)
	}
	return res, d
}

func (d doctorJSON) client(name string) (c struct {
	Client       string `json:"client"`
	Detected     bool   `json:"detected"`
	ConfigPath   string `json:"config_path"`
	EntryPresent bool   `json:"entry_present"`
	CommandOK    bool   `json:"command_ok"`
}) {
	for _, x := range d.Clients {
		if x.Client == name {
			return x
		}
	}
	return c
}

func TestMCPDoctorHealthyInstall(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, ".cursor"))
	env := installEnv(t, home)
	runE2E(t, t.TempDir(), env, "mcp", "install", "--client", "cursor").requireCode(t, 0)

	res, d := runDoctor(t, home)
	res.requireCode(t, 0)
	c := d.client("cursor")
	if !d.OK || !d.Handshake.OK || d.Handshake.ServerName == "" || !c.Detected || !c.EntryPresent || !c.CommandOK {
		t.Fatalf("unexpected doctor report: %s", res.stdout)
	}

	text := runE2E(t, t.TempDir(), env, "mcp", "doctor")
	text.requireCode(t, 0)
	if !strings.Contains(text.stdout, "[OK]") || !strings.Contains(text.stdout, "handshake") {
		t.Fatalf("expected [OK] lines and handshake, got:\n%s", text.stdout)
	}
}

func TestMCPDoctorBrokenCommandPathFails(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".cursor", "mcp.json"),
		`{"mcpServers":{"flutree":{"type":"stdio","command":"/nonexistent/flutree","args":["mcp","serve"]}}}`)

	res, d := runDoctor(t, home)
	res.requireCode(t, 1)
	c := d.client("cursor")
	if d.OK || !c.EntryPresent || c.CommandOK {
		t.Fatalf("expected broken command to be reported: %s", res.stdout)
	}
	text := runE2E(t, t.TempDir(), installEnv(t, home), "mcp", "doctor")
	text.requireCode(t, 1)
	if !strings.Contains(text.stdout, "[ERR]  cursor") {
		t.Fatalf("expected [ERR] for broken command, got:\n%s", text.stdout)
	}
}

// doctor must fail (exit 1) when a client's config cannot be read.
func TestMCPDoctorFailsOnUnreadableClientConfig(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), "{not json")

	res := runE2E(t, home, testEnv(home), "mcp", "doctor")
	res.requireCode(t, 1)
	if !strings.Contains(res.stdout, "[ERR]  cursor:") {
		t.Fatalf("expected an [ERR] line for cursor, got:\n%s", res.stdout)
	}
}

func openCodeFile(home string) string {
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

func objAt(t *testing.T, v any, what string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %v", what, v)
	}
	return m
}

// OpenCode (current docs, "V2") keeps servers under mcp.servers.<name>:
// https://opencode.ai/docs/mcp-servers/
func TestMCPInstallOpenCodeServersLayoutIsKeptAndUninstalled(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, openCodeFile(home), `{"theme":"dark","mcp":{"timeout":5000,"servers":{"other":{"type":"local","command":["x"]}}}}`)
	env := installEnv(t, home)

	runE2E(t, t.TempDir(), env, "mcp", "install", "--client", "opencode").requireCode(t, 0)
	mcp := objAt(t, readJSON(t, openCodeFile(home))["mcp"], "mcp")
	servers := objAt(t, mcp["servers"], "mcp.servers")
	fl := objAt(t, servers["flutree"], "mcp.servers.flutree")
	if fl["type"] != "local" {
		t.Fatalf("entry = %v", fl)
	}
	if _, stray := mcp["flutree"]; stray {
		t.Fatalf("flutree must not be written directly under mcp: %v", mcp)
	}
	if _, ok := servers["other"]; !ok || mcp["timeout"] == nil {
		t.Fatalf("existing config lost: %v", mcp)
	}

	dres, rep := runDoctor(t, home)
	dres.requireCode(t, 0)
	if c := rep.client("opencode"); !c.EntryPresent || !c.CommandOK {
		t.Fatalf("doctor must find the mcp.servers entry: %s", dres.stdout)
	}

	res := runE2E(t, t.TempDir(), env, "mcp", "uninstall", "--client", "opencode")
	res.requireCode(t, 0)
	if !strings.Contains(res.stdout, "removed") {
		t.Fatalf("expected removed, got %s", res.stdout)
	}
	mcp = objAt(t, readJSON(t, openCodeFile(home))["mcp"], "mcp")
	servers = objAt(t, mcp["servers"], "mcp.servers")
	if _, ok := servers["flutree"]; ok {
		t.Fatalf("flutree not removed: %v", servers)
	}
	if _, ok := servers["other"]; !ok || mcp["timeout"] == nil {
		t.Fatalf("uninstall removed more than flutree: %v", mcp)
	}
}

func TestMCPInstallOpenCodeLegacyLayoutIsKept(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, openCodeFile(home), `{"mcp":{"other":{"type":"local","command":["x"],"enabled":true}}}`)
	env := installEnv(t, home)

	runE2E(t, t.TempDir(), env, "mcp", "install", "--client", "opencode").requireCode(t, 0)
	mcp := objAt(t, readJSON(t, openCodeFile(home))["mcp"], "mcp")
	if _, ok := mcp["flutree"]; !ok {
		t.Fatalf("legacy layout must keep mcp.flutree: %v", mcp)
	}
	if _, ok := mcp["servers"]; ok {
		t.Fatalf("legacy layout must not gain mcp.servers: %v", mcp)
	}

	res := runE2E(t, t.TempDir(), env, "mcp", "uninstall", "--client", "opencode")
	res.requireCode(t, 0)
	if !strings.Contains(res.stdout, "removed") {
		t.Fatalf("expected removed, got %s", res.stdout)
	}
	mcp = objAt(t, readJSON(t, openCodeFile(home))["mcp"], "mcp")
	if _, ok := mcp["flutree"]; ok {
		t.Fatalf("flutree not removed: %v", mcp)
	}
	if _, ok := mcp["other"]; !ok {
		t.Fatalf("uninstall removed more than flutree: %v", mcp)
	}
}

func TestMCPInstallOpenCodeNewFileUsesServersLayout(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, ".config", "opencode"))
	runE2E(t, t.TempDir(), installEnv(t, home), "mcp", "install", "--client", "opencode").requireCode(t, 0)
	mcp := objAt(t, readJSON(t, openCodeFile(home))["mcp"], "mcp")
	servers := objAt(t, mcp["servers"], "mcp.servers")
	if _, ok := servers["flutree"]; !ok {
		t.Fatalf("new file must use mcp.servers.flutree: %v", mcp)
	}
	if _, ok := mcp["flutree"]; ok {
		t.Fatalf("new file must not use mcp.flutree: %v", mcp)
	}
}

// VS Code mcp.json may hold comments and trailing commas.
const vscodeJSONC = `{
  // my servers
  "servers": {
    "other": {"type": "stdio", "command": "x",},
  },
}
`

func TestMCPVSCodeJSONC(t *testing.T) {
	home := t.TempDir()
	file := filepath.Join(appDir(home, "Code"), "User", "mcp.json")
	mustWrite(t, file, vscodeJSONC)
	env := installEnv(t, home)

	// Install refuses to drop the comments, and leaves the file untouched.
	res := runE2E(t, t.TempDir(), env, "mcp", "install", "--client", "vscode")
	if !strings.Contains(res.stdout+res.stderr, "--force") {
		t.Fatalf("expected a --force hint, got stdout=%s stderr=%s", res.stdout, res.stderr)
	}
	if b, _ := os.ReadFile(file); string(b) != vscodeJSONC {
		t.Fatalf("file changed without --force:\n%s", b)
	}

	// Doctor parses the file (no entry yet is not a failure).
	res, d := runDoctor(t, home)
	res.requireCode(t, 0)
	if c := d.client("vscode"); !c.Detected || c.EntryPresent {
		t.Fatalf("doctor must read the JSONC file: %s", res.stdout)
	}

	// --force strips comments and writes.
	runE2E(t, t.TempDir(), env, "mcp", "install", "--client", "vscode", "--force").requireCode(t, 0)
	assertEntry(t, readJSON(t, file), "servers", true)

	// Doctor finds the entry.
	res, d = runDoctor(t, home)
	res.requireCode(t, 0)
	if c := d.client("vscode"); !c.EntryPresent || !c.CommandOK {
		t.Fatalf("doctor must find the entry: %s", res.stdout)
	}

	// Commented file with an entry: doctor reads it, uninstall refuses to rewrite.
	withEntry := `{
  // keep me
  "servers": {"flutree": {"type":"stdio","command":"` + os.Args[0] + `","args":["mcp","serve"]},},
}
`
	mustWrite(t, file, withEntry)
	res, d = runDoctor(t, home)
	if c := d.client("vscode"); !c.EntryPresent {
		t.Fatalf("doctor must find the entry in a JSONC file: %s", res.stdout)
	}
	res = runE2E(t, t.TempDir(), env, "mcp", "uninstall", "--client", "vscode")
	if !strings.Contains(res.stdout+res.stderr, "by hand") {
		t.Fatalf("expected manual-removal hint, got stdout=%s stderr=%s", res.stdout, res.stderr)
	}
	if b, _ := os.ReadFile(file); string(b) != withEntry {
		t.Fatalf("uninstall rewrote a commented file:\n%s", b)
	}
}

func TestMCPDoctorMissingEntryIsInformational(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, ".cursor"))
	text := runE2E(t, t.TempDir(), installEnv(t, home), "mcp", "doctor")
	text.requireCode(t, 0)
	if !strings.Contains(text.stdout, "[-]    cursor: no flutree entry (") {
		t.Fatalf("expected an informational [-] line, got:\n%s", text.stdout)
	}
	if strings.Contains(text.stdout, "[WARN] cursor") {
		t.Fatalf("missing entry must not warn:\n%s", text.stdout)
	}
}
