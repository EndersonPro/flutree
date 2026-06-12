package mcpinstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
