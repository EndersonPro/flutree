package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeConfigPort struct {
	loadDoc  domain.UserConfigDocument
	loadErr  error
	savedDoc domain.UserConfigDocument
	saveErr  error
	saved    bool
}

func (f *fakeConfigPort) Load() (domain.UserConfigDocument, error) {
	if f.loadErr != nil {
		return domain.UserConfigDocument{}, f.loadErr
	}
	return f.loadDoc, nil
}

func (f *fakeConfigPort) Save(doc domain.UserConfigDocument) error {
	f.saved = true
	f.savedDoc = doc
	return f.saveErr
}

func TestConfigServiceSetGetScopeRootRoundTrip(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	store := &fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1}}
	svc := NewConfigService(store)

	stored, err := svc.SetScopeRoot("scope.root", root)
	if err != nil {
		t.Fatalf("set scope.root failed: %v", err)
	}
	if stored != root {
		t.Fatalf("expected normalized root %q, got %q", root, stored)
	}
	if !store.saved {
		t.Fatalf("expected save call")
	}
	if store.savedDoc.Scope.Root != root {
		t.Fatalf("expected persisted root %q, got %q", root, store.savedDoc.Scope.Root)
	}

	store.loadDoc = store.savedDoc
	got, err := svc.GetScopeRoot("scope.root")
	if err != nil {
		t.Fatalf("get scope.root failed: %v", err)
	}
	if got != root {
		t.Fatalf("expected %q, got %q", root, got)
	}
}

func TestConfigServiceRejectsUnsupportedKey(t *testing.T) {
	store := &fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1}}
	svc := NewConfigService(store)

	if _, err := svc.SetScopeRoot("scope.invalid", t.TempDir()); err == nil {
		t.Fatalf("expected unsupported key error on set")
	}
	if _, err := svc.GetScopeRoot("scope.invalid"); err == nil {
		t.Fatalf("expected unsupported key error on get")
	}
}

func TestConfigServiceSetRejectsInvalidPathAndDoesNotMutate(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	store := &fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1}}
	svc := NewConfigService(store)

	_, err := svc.SetScopeRoot("scope.root", missing)
	if err == nil {
		t.Fatalf("expected invalid path error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "does not exist") {
		t.Fatalf("expected does-not-exist validation, got: %v", err)
	}
	if store.saved {
		t.Fatalf("save should not be called when validation fails")
	}
}

func TestConfigServiceSetRejectsNonDirectoryPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1}}
	svc := NewConfigService(store)

	_, err := svc.SetScopeRoot("scope.root", file)
	if err == nil {
		t.Fatalf("expected non-directory error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "directory") {
		t.Fatalf("expected non-directory validation error, got: %v", err)
	}
	if store.saved {
		t.Fatalf("save should not be called for non-directory path")
	}
}

func TestConfigServiceSetRejectsUnreachableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not reliable on windows")
	}

	parent := t.TempDir()
	unreachable := filepath.Join(parent, "private")
	if err := os.MkdirAll(unreachable, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreachable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreachable, 0o700) })

	store := &fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1}}
	svc := NewConfigService(store)

	_, err := svc.SetScopeRoot("scope.root", unreachable)
	if err == nil {
		t.Fatalf("expected unreachable path error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "reachable") {
		t.Fatalf("expected reachable-path validation, got: %v", err)
	}
	if store.saved {
		t.Fatalf("save should not be called for unreachable path")
	}
}
