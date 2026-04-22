package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

func TestRepositoryLoadCreatesDefaultDocument(t *testing.T) {
	repo := &Repository{Path: filepath.Join(t.TempDir(), ".flutree_config.json")}

	first, err := repo.Load()
	if err != nil {
		t.Fatalf("load should create default document: %v", err)
	}
	second, err := repo.Load()
	if err != nil {
		t.Fatalf("second load should also succeed: %v", err)
	}

	if first.Version != 1 || second.Version != 1 {
		t.Fatalf("expected version 1 defaults, got first=%d second=%d", first.Version, second.Version)
	}
	if first.Scope.Root != "" || second.Scope.Root != "" {
		t.Fatalf("expected empty scope root on default document, got first=%q second=%q", first.Scope.Root, second.Scope.Root)
	}
}

func TestRepositorySaveLoadRoundtrip(t *testing.T) {
	repo := &Repository{Path: filepath.Join(t.TempDir(), ".flutree_config.json")}
	want := domain.UserConfigDocument{
		Version: 1,
		Scope:   domain.UserScopeConfig{Root: filepath.Clean(t.TempDir())},
	}

	if err := repo.Save(want); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := repo.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got.Version != want.Version || got.Scope.Root != want.Scope.Root {
		t.Fatalf("unexpected persisted value: got=%+v want=%+v", got, want)
	}
}

func TestRepositoryLoadRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".flutree_config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &Repository{Path: path}
	_, err := repo.Load()
	if err == nil {
		t.Fatalf("expected parse error for malformed json")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "parse") {
		t.Fatalf("expected parse error message, got: %v", err)
	}
}

func TestRepositoryLoadRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".flutree_config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]any{"version": 999, "scope": map[string]any{"root": "/tmp"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &Repository{Path: path}
	_, err = repo.Load()
	if err == nil {
		t.Fatalf("expected unsupported version error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unsupported") {
		t.Fatalf("expected unsupported version in error, got: %v", err)
	}
}

func TestRepositorySaveUsesAtomicReplaceForReaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".flutree_config.json")
	repo := &Repository{Path: path}

	if err := repo.Save(domain.UserConfigDocument{Version: 1}); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	stop := make(chan struct{})

	reader := func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				b, err := os.ReadFile(path)
				if err != nil {
					errCh <- err
					return
				}
				var doc domain.UserConfigDocument
				if err := json.Unmarshal(b, &doc); err != nil {
					errCh <- err
					return
				}
			}
		}
	}

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go reader()
	}

	for i := 0; i < 40; i++ {
		root := filepath.Join(t.TempDir(), "scope", string(rune('a'+(i%26))))
		if err := repo.Save(domain.UserConfigDocument{Version: 1, Scope: domain.UserScopeConfig{Root: root}}); err != nil {
			t.Fatalf("save iteration %d failed: %v", i, err)
		}
	}

	close(stop)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("reader observed invalid intermediate state: %v", err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file should not remain after save, stat err=%v", err)
	}
}
