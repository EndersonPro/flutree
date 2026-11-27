package mcpinstall

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// commandExecutable reports whether cmd resolves to an existing executable
// file, either as a path or through PATH.
func commandExecutable(cmd string) bool {
	if cmd == "" {
		return false
	}
	if !strings.ContainsRune(cmd, filepath.Separator) {
		_, err := exec.LookPath(cmd)
		return err == nil
	}
	info, err := os.Stat(cmd)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// BinaryWarning returns a warning when the binary lives in a temporary
// directory or looks like a `go run` build, so an installed entry would break.
func BinaryWarning(path string) string {
	if strings.Contains(path, "go-build") {
		return "binary looks like a `go run` build; install flutree and re-run"
	}
	tmp := filepath.Clean(os.TempDir())
	if strings.HasPrefix(filepath.Clean(path), tmp+string(filepath.Separator)) {
		return "binary lives in a temporary directory (" + tmp + "); it may disappear"
	}
	return ""
}

// HandshakeResult is the outcome of a live `mcp serve` initialize exchange.
type HandshakeResult struct {
	OK            bool   `json:"ok"`
	ServerName    string `json:"server_name,omitempty"`
	ServerVersion string `json:"server_version,omitempty"`
	Error         string `json:"error,omitempty"`
}

// Handshake spawns `<binary> mcp serve`, sends initialize over stdio and
// expects serverInfo in the reply within timeout.
func Handshake(binary string, timeout time.Duration) HandshakeResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "mcp", "serve")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return HandshakeResult{Error: err.Error()}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return HandshakeResult{Error: err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return HandshakeResult{Error: "spawn: " + err.Error()}
	}
	defer func() {
		_ = stdin.Close()
		cancel()
		_ = cmd.Wait()
	}()

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"flutree-doctor","version":"1.0.0"}}}` + "\n"
	if _, err := stdin.Write([]byte(req)); err != nil {
		return HandshakeResult{Error: "write initialize: " + err.Error()}
	}

	type reply struct {
		line string
		err  error
	}
	ch := make(chan reply, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		ch <- reply{line, err}
	}()
	select {
	case <-ctx.Done():
		return HandshakeResult{Error: fmt.Sprintf("no initialize reply within %s", timeout)}
	case r := <-ch:
		if r.err != nil && r.line == "" {
			return HandshakeResult{Error: "read reply: " + r.err.Error()}
		}
		var resp struct {
			Result struct {
				ServerInfo struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"serverInfo"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(r.line), &resp); err != nil || resp.Result.ServerInfo.Name == "" {
			return HandshakeResult{Error: "reply has no serverInfo: " + strings.TrimSpace(r.line)}
		}
		return HandshakeResult{OK: true, ServerName: resp.Result.ServerInfo.Name, ServerVersion: resp.Result.ServerInfo.Version}
	}
}
