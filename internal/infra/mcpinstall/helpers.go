package mcpinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// loadJSONMap reads a file as map[string]json.RawMessage.
// If the file does not exist, it returns an empty map (caller creates the file).
// If the file exists but cannot be decoded, it returns an error.
func loadJSONMap(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
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
// It writes to a sibling .tmp file then renames to provide atomic replacement.
func writeJSONMap(path string, m map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, data, 0o644)
}

// atomicWrite writes data to path using a temp-file + rename so concurrent
// readers never see a partial write.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
