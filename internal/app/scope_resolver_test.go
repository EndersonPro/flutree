package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

func TestScopeResolverPrefersExplicitFlag(t *testing.T) {
	explicit := filepath.Clean(t.TempDir())
	persisted := filepath.Clean(t.TempDir())

	resolver := NewScopeResolver(&fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1, Scope: domain.UserScopeConfig{Root: persisted}}})
	got, err := resolver.Resolve(explicit, true)
	if err != nil {
		t.Fatalf("resolve with explicit flag should succeed: %v", err)
	}
	if got != explicit {
		t.Fatalf("expected explicit scope %q, got %q", explicit, got)
	}
}

func TestScopeResolverUsesPersistedWhenFlagOmitted(t *testing.T) {
	persisted := filepath.Clean(t.TempDir())

	resolver := NewScopeResolver(&fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1, Scope: domain.UserScopeConfig{Root: persisted}}})
	got, err := resolver.Resolve(".", false)
	if err != nil {
		t.Fatalf("resolve without explicit flag should use persisted root: %v", err)
	}
	if got != persisted {
		t.Fatalf("expected persisted scope %q, got %q", persisted, got)
	}
}

func TestScopeResolverFallsBackToDotWhenPersistedEmpty(t *testing.T) {
	resolver := NewScopeResolver(&fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1}})
	got, err := resolver.Resolve(".", false)
	if err != nil {
		t.Fatalf("resolve fallback should succeed: %v", err)
	}
	want, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("expected fallback scope %q, got %q", want, got)
	}
}

func TestScopeResolverReturnsClearErrorForInvalidPersistedPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	resolver := NewScopeResolver(&fakeConfigPort{loadDoc: domain.UserConfigDocument{Version: 1, Scope: domain.UserScopeConfig{Root: missing}}})

	_, err := resolver.Resolve(".", false)
	if err == nil {
		t.Fatalf("expected invalid persisted path error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "persisted") {
		t.Fatalf("expected persisted-path context in error, got: %v", err)
	}
}
