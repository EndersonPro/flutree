package mcpinstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tidwall/jsonc"
)

// loadJSONMap reads a file as map[string]json.RawMessage.
// If the file does not exist or is empty (0 bytes), it returns an empty map
// (caller creates the file). If the file exists but cannot be decoded, it
// returns an error.
func loadJSONMap(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	// Empty file: treat as fresh start (no data to lose).
	if len(data) == 0 {
		return map[string]json.RawMessage{}, nil
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("malformed JSON in %s: %w", filepath.Base(path), err)
	}
	return m, nil
}

// loadJSONCMap is loadJSONMap for configs that may carry JSONC comments and
// trailing commas. lossy reports that rewriting the file would drop them (the
// bytes change when normalised to plain JSON); callers may read such a file
// but must not rewrite it without an explicit --force.
func loadJSONCMap(path string) (root map[string]json.RawMessage, lossy bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]json.RawMessage{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return map[string]json.RawMessage{}, false, nil
	}
	plain := jsonc.ToJSONInPlace(append([]byte(nil), data...))
	if err := json.Unmarshal(plain, &root); err != nil {
		return nil, false, fmt.Errorf("malformed JSON in %s: %w", filepath.Base(path), err)
	}
	return root, string(plain) != string(data), nil
}

// getOrCreateRawMap returns the sub-map stored under key in root, creating an
// empty map if the key is absent.  It returns an error if the key exists but
// is not a JSON object.
func getOrCreateRawMap(root map[string]json.RawMessage, key string) (map[string]json.RawMessage, error) {
	raw, exists := root[key]
	if !exists {
		return map[string]json.RawMessage{}, nil
	}
	var sub map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sub); err != nil {
		return nil, fmt.Errorf("key %q is not a JSON object: %w", key, err)
	}
	return sub, nil
}

// writeJSONMap atomically writes a map[string]json.RawMessage as indented JSON.
// It preserves the file permissions of an existing file; new files are created
// with mode 0o600 to protect tokens from other users on multi-user systems.
func writeJSONMap(path string, m map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	// Preserve existing file permissions; default to 0o600 for new files.
	perm := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	return atomicWrite(path, data, perm)
}

// atomicWrite writes data to path using a temp-file + rename so concurrent
// readers never see a partial write.  The temp file is created with os.CreateTemp
// in the same directory as path to ensure the rename is always on the same
// filesystem.  The temp file is removed on error.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-mcpinstall-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Ensure cleanup on any error path.
	cleanup := func() { _ = os.Remove(tmpPath) }

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// entryName is the key under which every client stores the flutree server.
const entryName = "flutree"

// jsonRemoveEntry deletes only root[key][flutree] from the JSON file at path,
// preserving every other key and server. It returns OutcomeNotFound when the
// file or entry does not exist, and never writes in that case.
func jsonRemoveEntry(path, key string) (Outcome, error) {
	return jsonRemoveEntryOpt(path, key, false)
}

// jsonRemoveEntryOpt is jsonRemoveEntry for configs that may be JSONC. When
// the entry exists in a file with comments or trailing commas it refuses to
// rewrite (that would drop them) and asks for a manual removal.
func jsonRemoveEntryOpt(path, key string, allowJSONC bool) (Outcome, error) {
	var (
		root  map[string]json.RawMessage
		lossy bool
		err   error
	)
	if allowJSONC {
		root, lossy, err = loadJSONCMap(path)
	} else {
		root, err = loadJSONMap(path)
	}
	if err != nil {
		return OutcomeError, err
	}
	servers, err := getOrCreateRawMap(root, key)
	if err != nil {
		return OutcomeError, err
	}
	if _, ok := servers[entryName]; !ok {
		return OutcomeNotFound, nil
	}
	if lossy {
		return OutcomeError, fmt.Errorf("config contains JSONC comments or trailing commas; remove the %q entry from %s by hand", entryName, path)
	}
	delete(servers, entryName)
	raw, err := json.Marshal(servers)
	if err != nil {
		return OutcomeError, err
	}
	root[key] = json.RawMessage(raw)
	if err := writeJSONMap(path, root); err != nil {
		return OutcomeError, err
	}
	return OutcomeRemoved, nil
}

// jsonLookupCommand returns the command of root[key][flutree] in the JSON file.
func jsonLookupCommand(path, key string) (string, bool, error) {
	return jsonLookupCommandOpt(path, key, false)
}

func jsonLookupCommandOpt(path, key string, allowJSONC bool) (string, bool, error) {
	var (
		root map[string]json.RawMessage
		err  error
	)
	if allowJSONC {
		root, _, err = loadJSONCMap(path)
	} else {
		root, err = loadJSONMap(path)
	}
	if err != nil {
		return "", false, err
	}
	servers, err := getOrCreateRawMap(root, key)
	if err != nil {
		return "", false, err
	}
	raw, ok := servers[entryName]
	if !ok {
		return "", false, nil
	}
	return commandOf(raw), true, nil
}

// commandOf extracts the executable from a server entry whose "command" is
// either a string or an array whose first element is the executable.
func commandOf(raw json.RawMessage) string {
	var e struct {
		Command json.RawMessage `json:"command"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(e.Command, &s) == nil {
		return s
	}
	var arr []string
	if json.Unmarshal(e.Command, &arr) == nil && len(arr) > 0 {
		return arr[0]
	}
	return ""
}

// homeOf resolves the home directory, honoring an injected base for tests.
func homeOf(baseDir string) (string, error) {
	if baseDir != "" {
		return baseDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return home, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func previewJSON(entry any) (string, error) {
	data, err := json.MarshalIndent(map[string]any{entryName: entry}, "", "  ")
	return string(data), err
}
