package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeAddRepoGit struct {
	discovered       []domain.DiscoveredFlutterRepo
	branchExists     map[string]bool
	syncBaseCalls    []string
	syncBaseErr      error
	syncBranchCalls  []string
	createNewCalls   []string
	createExistCalls []string
	removed          []string
}

func (f *fakeAddRepoGit) EnsureRepo() (string, error) { return "", nil }
func (f *fakeAddRepoGit) ListWorktrees(string) ([]domain.GitWorktreeEntry, error) {
	return nil, nil
}
func (f *fakeAddRepoGit) CreateWorktree(string, string, string, string) error { return nil }
func (f *fakeAddRepoGit) CreateWorktreeNew(repoRoot, path, branch, startPoint string) error {
	f.createNewCalls = append(f.createNewCalls, repoRoot+"::"+path+"::"+branch+"::"+startPoint)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "pubspec.yaml"), []byte("name: core\n"), 0o644)
}
func (f *fakeAddRepoGit) CreateWorktreeExisting(repoRoot, path, branch string) error {
	f.createExistCalls = append(f.createExistCalls, repoRoot+"::"+path+"::"+branch)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "pubspec.yaml"), []byte("name: core\n"), 0o644)
}
func (f *fakeAddRepoGit) BranchExists(repoRoot, branch string) (bool, error) {
	if f.branchExists == nil {
		return false, nil
	}
	return f.branchExists[repoRoot+"::"+branch], nil
}
func (f *fakeAddRepoGit) RemoteBranchExists(repoRoot, branch string) (bool, error) {
	return false, nil
}
func (f *fakeAddRepoGit) SyncBranchWithRemote(repoRoot, branch string) error {
	f.syncBranchCalls = append(f.syncBranchCalls, repoRoot+"::"+branch)
	return nil
}
func (f *fakeAddRepoGit) SyncBaseBranch(repoRoot, baseBranch string) (string, error) {
	f.syncBaseCalls = append(f.syncBaseCalls, repoRoot+"::"+baseBranch)
	if f.syncBaseErr != nil {
		return "", f.syncBaseErr
	}
	return "origin/" + baseBranch, nil
}
func (f *fakeAddRepoGit) RemoveWorktree(repoRoot, path string, force bool) error {
	f.removed = append(f.removed, repoRoot+"::"+path)
	return nil
}
func (f *fakeAddRepoGit) IsDirty(string) (bool, error) { return false, nil }
func (f *fakeAddRepoGit) DiscoverFlutterRepos(scope string) ([]domain.DiscoveredFlutterRepo, error) {
	return append([]domain.DiscoveredFlutterRepo{}, f.discovered...), nil
}

type fakeAddRepoRegistry struct {
	records []domain.RegistryRecord
	upserts []domain.RegistryRecord
}

func (f *fakeAddRepoRegistry) ListRecords() ([]domain.RegistryRecord, error) {
	return append([]domain.RegistryRecord{}, f.records...), nil
}
func (f *fakeAddRepoRegistry) Get(name string) (domain.RegistryRecord, error) {
	for _, rec := range f.records {
		if rec.Name == name {
			return rec, nil
		}
	}
	return domain.RegistryRecord{}, errors.New("not found")
}
func (f *fakeAddRepoRegistry) Upsert(record domain.RegistryRecord) error {
	f.upserts = append(f.upserts, record)
	f.records = append(f.records, record)
	return nil
}
func (f *fakeAddRepoRegistry) Remove(name string) (domain.RegistryRecord, error) {
	return domain.RegistryRecord{Name: name}, nil
}
func (f *fakeAddRepoRegistry) MarkCompleted(name string) (domain.RegistryRecord, error) {
	return domain.RegistryRecord{Name: name}, nil
}

type fakeAddRepoPrompt struct {
	askTextResponses []string
	confirmResponses []bool
	selectPackages   []string
	askTextCalls     []string
	confirmCalls     []string
	askTextErr       error
	confirmErr       error
	selectErr        error
}

func (f *fakeAddRepoPrompt) Confirm(message string, nonInteractive, assumeYes bool) (bool, error) {
	f.confirmCalls = append(f.confirmCalls, message)
	if f.confirmErr != nil {
		return false, f.confirmErr
	}
	if len(f.confirmResponses) == 0 {
		return false, nil
	}
	resp := f.confirmResponses[0]
	f.confirmResponses = f.confirmResponses[1:]
	return resp, nil
}
func (f *fakeAddRepoPrompt) ConfirmWithToken(message, token string, nonInteractive, assumeYes bool) (bool, error) {
	return true, nil
}
func (f *fakeAddRepoPrompt) SelectOne(message string, choices []string, nonInteractive bool) (string, error) {
	return "", nil
}
func (f *fakeAddRepoPrompt) SelectPackages(message string, choices []string, nonInteractive bool) ([]string, error) {
	if f.selectErr != nil {
		return nil, f.selectErr
	}
	return append([]string{}, f.selectPackages...), nil
}
func (f *fakeAddRepoPrompt) AskText(message, defaultValue string, nonInteractive bool) (string, error) {
	f.askTextCalls = append(f.askTextCalls, message+"::"+defaultValue)
	if f.askTextErr != nil {
		return "", f.askTextErr
	}
	if len(f.askTextResponses) == 0 {
		return defaultValue, nil
	}
	resp := f.askTextResponses[0]
	f.askTextResponses = f.askTextResponses[1:]
	return resp, nil
}

func TestAddRepoNonInteractiveDefaultsWithoutPrompts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(t.TempDir(), "root-repo")
	pkgRoot := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: pkgRoot, PackageName: "core"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: repoRoot, Status: "active",
	}}}
	prompt := &fakeAddRepoPrompt{}

	service := NewAddRepoService(git, registry, prompt)
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:     "feature-login",
		ExecutionScope:    ".",
		RepoSelectors:     []string{"core-pkg"},
		SyncPolicy:        domain.AddRepoSyncAuto,
		NonInteractive:    true,
		RootFiles:         nil,
		PackageBaseBranch: map[string]string{},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(prompt.askTextCalls) != 0 {
		t.Fatalf("expected no AskText prompts in non-interactive mode, got=%v", prompt.askTextCalls)
	}
	if len(prompt.confirmCalls) != 0 {
		t.Fatalf("expected no Confirm prompts in non-interactive mode, got=%v", prompt.confirmCalls)
	}
	if len(git.createNewCalls) != 1 {
		t.Fatalf("expected one create-new call, got=%v", git.createNewCalls)
	}
	if !strings.Contains(git.createNewCalls[0], "::feature/login::main") {
		t.Fatalf("expected default branch/base feature/login+main, got=%s", git.createNewCalls[0])
	}
}

func TestAddRepoInteractivePromptsForMissingBranchAndBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(t.TempDir(), "root-repo")
	pkgRoot := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: pkgRoot, PackageName: "core"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: repoRoot, Status: "active",
	}}}
	prompt := &fakeAddRepoPrompt{askTextResponses: []string{"feature/core", "release/1.2"}, confirmResponses: []bool{false}}

	service := NewAddRepoService(git, registry, prompt)
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:  "feature-login",
		ExecutionScope: ".",
		RepoSelectors:  []string{"core-pkg"},
		SyncPolicy:     domain.AddRepoSyncAuto,
		NonInteractive: false,
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(prompt.askTextCalls) != 2 {
		t.Fatalf("expected branch/base prompts, got=%v", prompt.askTextCalls)
	}
	if len(git.createNewCalls) != 1 {
		t.Fatalf("expected one create-new call, got=%v", git.createNewCalls)
	}
	if !strings.Contains(git.createNewCalls[0], "::feature/core::release/1.2") {
		t.Fatalf("expected prompted branch/base in call, got=%s", git.createNewCalls[0])
	}
}

func TestAddRepoSyncPolicyAlwaysPropagatesToExistingBranchFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(t.TempDir(), "root-repo")
	pkgRoot := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{
		discovered: []domain.DiscoveredFlutterRepo{
			{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
			{Name: "core-pkg", RepoRoot: pkgRoot, PackageName: "core"},
		},
		branchExists: map[string]bool{pkgRoot + "::feature/login": true},
	}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: repoRoot, Status: "active",
	}}}
	prompt := &fakeAddRepoPrompt{}

	service := NewAddRepoService(git, registry, prompt)
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:       "feature-login",
		ExecutionScope:      ".",
		RepoSelectors:       []string{"core-pkg"},
		PackageBaseBranch:   map[string]string{"core-pkg": "main"},
		PackageBranchSource: map[string]string{"core-pkg": "feature/login"},
		SyncPolicy:          domain.AddRepoSyncAlways,
		ReuseExistingBranch: true,
		NonInteractive:      true,
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(git.syncBranchCalls) != 1 || git.syncBranchCalls[0] != pkgRoot+"::feature/login" {
		t.Fatalf("expected sync existing branch call, got=%v", git.syncBranchCalls)
	}
	if len(git.createExistCalls) != 1 || !strings.Contains(git.createExistCalls[0], "::feature/login") {
		t.Fatalf("expected create-existing with feature/login, got=%v", git.createExistCalls)
	}
}

func TestAddRepoSyncFailureReturnsActionableErrorAndRollsBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(t.TempDir(), "root-repo")
	pkgRoot := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{
		discovered: []domain.DiscoveredFlutterRepo{
			{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
			{Name: "core-pkg", RepoRoot: pkgRoot, PackageName: "core"},
		},
		syncBaseErr: errors.New("origin unreachable"),
	}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: repoRoot, Status: "active",
	}}}
	prompt := &fakeAddRepoPrompt{}

	service := NewAddRepoService(git, registry, prompt)
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:       "feature-login",
		ExecutionScope:      ".",
		RepoSelectors:       []string{"core-pkg"},
		PackageBaseBranch:   map[string]string{"core-pkg": "main"},
		PackageBranchSource: map[string]string{"core-pkg": "feature/core"},
		SyncPolicy:          domain.AddRepoSyncAlways,
		NonInteractive:      true,
	})
	if err == nil {
		t.Fatalf("expected sync failure")
	}
	if !strings.Contains(err.Error(), "core-pkg") {
		t.Fatalf("expected actionable repo context in error, got=%v", err)
	}
	if len(git.createNewCalls) != 0 {
		t.Fatalf("expected no worktree creation when sync fails, got=%v", git.createNewCalls)
	}
	if len(registry.upserts) != 0 {
		t.Fatalf("expected no registry writes when sync fails, got=%v", registry.upserts)
	}
}

func TestAddRepoNonInteractiveFailsWhenRootBranchMetadataIsMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(t.TempDir(), "root-repo")
	pkgRoot := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: pkgRoot, PackageName: "core"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "   ", Path: rootPath, RepoRoot: repoRoot, Status: "active",
	}}}
	prompt := &fakeAddRepoPrompt{}

	service := NewAddRepoService(git, registry, prompt)
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:  "feature-login",
		ExecutionScope: ".",
		RepoSelectors:  []string{"core-pkg"},
		SyncPolicy:     domain.AddRepoSyncAuto,
		NonInteractive: true,
	})
	if err == nil {
		t.Fatalf("expected non-interactive validation error for unresolved branch metadata")
	}
	if !strings.Contains(err.Error(), "Cannot resolve target branch") {
		t.Fatalf("expected unresolved branch validation message, got=%v", err)
	}
	if len(git.createNewCalls) != 0 {
		t.Fatalf("expected fail-before-mutation with no worktree creation, got=%v", git.createNewCalls)
	}
	if len(registry.upserts) != 0 {
		t.Fatalf("expected no registry writes on validation failure, got=%v", registry.upserts)
	}
	if len(prompt.askTextCalls) != 0 || len(prompt.confirmCalls) != 0 {
		t.Fatalf("expected no prompts in non-interactive validation failure, ask=%v confirm=%v", prompt.askTextCalls, prompt.confirmCalls)
	}
}

func TestAddRepoInteractiveWithPackageBaseSkipsBranchAndBasePrompts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(t.TempDir(), "root-repo")
	pkgRoot := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: pkgRoot, PackageName: "core"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: repoRoot, Status: "active",
	}}}
	prompt := &fakeAddRepoPrompt{}

	service := NewAddRepoService(git, registry, prompt)
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:     "feature-login",
		ExecutionScope:    ".",
		RepoSelectors:     []string{"core-pkg"},
		PackageBaseBranch: map[string]string{"core-pkg": "main"},
		SyncPolicy:        domain.AddRepoSyncNever,
		NonInteractive:    false,
	})
	if err != nil {
		t.Fatalf("expected success with explicit package-base compatibility path, got: %v", err)
	}
	if len(prompt.askTextCalls) != 0 {
		t.Fatalf("expected no branch/base prompts when --package-base is provided, got=%v", prompt.askTextCalls)
	}
	if len(git.createNewCalls) != 1 {
		t.Fatalf("expected one create-new call, got=%v", git.createNewCalls)
	}
	if !strings.Contains(git.createNewCalls[0], "::feature/login::main") {
		t.Fatalf("expected root branch + explicit package-base in call, got=%s", git.createNewCalls[0])
	}
}

func TestAddRepoNonInteractiveRequiresRepoSelectors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(t.TempDir(), "root-repo")
	pkgRoot := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: pkgRoot, PackageName: "core"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: repoRoot, Status: "active",
	}}}
	prompt := &fakeAddRepoPrompt{}

	service := NewAddRepoService(git, registry, prompt)
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:  "feature-login",
		ExecutionScope: ".",
		NonInteractive: true,
	})
	if err == nil {
		t.Fatalf("expected missing selector validation error")
	}
	if !strings.Contains(err.Error(), "Repository selection is required") {
		t.Fatalf("expected missing selector guidance, got=%v", err)
	}
	if len(prompt.selectPackages) != 0 || len(prompt.askTextCalls) != 0 || len(prompt.confirmCalls) != 0 {
		t.Fatalf("expected no prompts in non-interactive mode")
	}
}

func TestAddRepoUnknownSelectorReturnsInputError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(t.TempDir(), "root-repo")
	pkgRoot := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: pkgRoot, PackageName: "core"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: repoRoot, Status: "active",
	}}}

	service := NewAddRepoService(git, registry, &fakeAddRepoPrompt{})
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:  "feature-login",
		ExecutionScope: ".",
		RepoSelectors:  []string{"missing"},
		NonInteractive: true,
	})
	if err == nil {
		t.Fatalf("expected unknown selector error")
	}
	if !strings.Contains(err.Error(), "Unknown --repo selector") {
		t.Fatalf("expected selector error, got=%v", err)
	}
}

func TestAddRepoMissingWorkspaceNameReturnsInputError(t *testing.T) {
	service := NewAddRepoService(&fakeAddRepoGit{}, &fakeAddRepoRegistry{}, &fakeAddRepoPrompt{})

	_, err := service.Run(domain.AddRepoInput{WorkspaceName: "   "})
	if err == nil {
		t.Fatalf("expected missing workspace validation error")
	}
	if !strings.Contains(err.Error(), "Missing workspace name") {
		t.Fatalf("expected missing workspace message, got=%v", err)
	}
}

func TestAddRepoRejectsPackageWorkspaceNameInput(t *testing.T) {
	service := NewAddRepoService(&fakeAddRepoGit{}, &fakeAddRepoRegistry{}, &fakeAddRepoPrompt{})

	_, err := service.Run(domain.AddRepoInput{WorkspaceName: "feature-login__pkg__core"})
	if err == nil {
		t.Fatalf("expected package-workspace validation error")
	}
	if !strings.Contains(err.Error(), "Add-repo requires root workspace name") {
		t.Fatalf("expected root workspace guidance, got=%v", err)
	}
}

func TestAddRepoFailsWhenWorkspaceNotInRegistry(t *testing.T) {
	git := &fakeAddRepoGit{}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{}}

	service := NewAddRepoService(git, registry, &fakeAddRepoPrompt{})
	_, err := service.Run(domain.AddRepoInput{WorkspaceName: "missing", ExecutionScope: "."})
	if err == nil {
		t.Fatalf("expected precondition error")
	}
	if !strings.Contains(err.Error(), "was not found in registry") {
		t.Fatalf("expected registry lookup failure, got=%v", err)
	}
}

func TestAddRepoFailsWhenRootRepoIsNotDiscoverable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootRepo := filepath.Join(t.TempDir(), "root-repo")
	otherRepo := filepath.Join(t.TempDir(), "other-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{{
		Name: "other", RepoRoot: otherRepo, PackageName: "other",
	}}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: rootRepo, Status: "active",
	}}}

	service := NewAddRepoService(git, registry, &fakeAddRepoPrompt{})
	_, err := service.Run(domain.AddRepoInput{WorkspaceName: "feature-login", ExecutionScope: ".", RepoSelectors: []string{"other"}, NonInteractive: true})
	if err == nil {
		t.Fatalf("expected root repo discoverability error")
	}
	if !strings.Contains(err.Error(), "Root repository is not discoverable") {
		t.Fatalf("expected root discoverability message, got=%v", err)
	}
}

func TestAddRepoReturnsNoCandidatesWhenAllReposAlreadyAttached(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	pkgPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "packages", "core-pkg")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pkgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootRepo := filepath.Join(t.TempDir(), "root-repo")
	pkgRepo := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: rootRepo, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: pkgRepo, PackageName: "core"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{
		{Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: rootRepo, Status: "active"},
		{Name: "feature-login__pkg__core-pkg", Branch: "feature/login", Path: pkgPath, RepoRoot: pkgRepo, Status: "active"},
	}}

	service := NewAddRepoService(git, registry, &fakeAddRepoPrompt{})
	_, err := service.Run(domain.AddRepoInput{WorkspaceName: "feature-login", ExecutionScope: ".", NonInteractive: true, RepoSelectors: []string{"core-pkg"}})
	if err == nil {
		t.Fatalf("expected no-candidates precondition error")
	}
	if !strings.Contains(err.Error(), "No additional repositories available") {
		t.Fatalf("expected no-candidates message, got=%v", err)
	}
}

func TestAddRepoInteractiveSelectorPromptMapsChoicesToRepoRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootRepo := filepath.Join(t.TempDir(), "root-repo")
	pkgRepo := filepath.Join(t.TempDir(), "core-repo")
	core := domain.DiscoveredFlutterRepo{Name: "core-pkg", RepoRoot: pkgRepo, PackageName: "core"}
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: rootRepo, PackageName: "root_app"},
		core,
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: rootRepo, Status: "active",
	}}}
	prompt := &fakeAddRepoPrompt{
		selectPackages:   []string{repoLabel(core)},
		askTextResponses: []string{"feature/core", "main"},
		confirmResponses: []bool{false},
	}

	service := NewAddRepoService(git, registry, prompt)
	_, err := service.Run(domain.AddRepoInput{WorkspaceName: "feature-login", ExecutionScope: ".", NonInteractive: false, SyncPolicy: domain.AddRepoSyncAuto})
	if err != nil {
		t.Fatalf("expected success with interactive repo selection, got=%v", err)
	}
	if len(git.createNewCalls) != 1 || !strings.Contains(git.createNewCalls[0], pkgRepo) {
		t.Fatalf("expected create-new call for selected repo root, got=%v", git.createNewCalls)
	}
}

func TestAddRepoUpdatesExistingWorkspaceFileWithAttachedRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	containerPath := filepath.Join(home, "Documents", "worktrees", "feature-login")
	rootPath := filepath.Join(containerPath, "root", "root-app")
	existingPkgPath := filepath.Join(containerPath, "packages", "core-pkg")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(existingPkgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existingPkgPath, "pubspec.yaml"), []byte("name: core\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	workspacePath := filepath.Join(containerPath, "feature-login.code-workspace")
	workspaceContent := "{\n  \"folders\": [\n    {\"path\": \"root/root-app\"},\n    {\"path\": \"packages/core-pkg\"}\n  ]\n}\n"
	if err := os.WriteFile(workspacePath, []byte(workspaceContent), 0o644); err != nil {
		t.Fatal(err)
	}

	rootRepo := filepath.Join(t.TempDir(), "root-repo")
	coreRepo := filepath.Join(t.TempDir(), "core-repo")
	designRepo := filepath.Join(t.TempDir(), "design-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: rootRepo, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: coreRepo, PackageName: "core"},
		{Name: "design-pkg", RepoRoot: designRepo, PackageName: "design"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{
		{Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: rootRepo, Status: "active"},
		{Name: "feature-login__pkg__core-pkg", Branch: "feature/login", Path: existingPkgPath, RepoRoot: coreRepo, Status: "active"},
	}}

	service := NewAddRepoService(git, registry, &fakeAddRepoPrompt{})
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:  "feature-login",
		ExecutionScope: ".",
		RepoSelectors:  []string{"design-pkg"},
		SyncPolicy:     domain.AddRepoSyncNever,
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("expected success, got=%v", err)
	}

	content, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatalf("expected workspace file to exist, got=%v", err)
	}

	var payload struct {
		Folders []struct {
			Path string `json:"path"`
		} `json:"folders"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("expected valid workspace JSON, got=%v", err)
	}

	got := make([]string, 0, len(payload.Folders))
	for _, folder := range payload.Folders {
		got = append(got, folder.Path)
	}
	want := []string{"root/root-app", "packages/core-pkg", "packages/design-pkg"}
	if len(got) != len(want) {
		t.Fatalf("unexpected folder count. got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected folder order/content. got=%v want=%v", got, want)
		}
	}
}

func TestAddRepoSkipsWorkspaceUpdateWhenWorkspaceFileIsMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	containerPath := filepath.Join(home, "Documents", "worktrees", "feature-login")
	rootPath := filepath.Join(containerPath, "root", "root-app")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "pubspec.yaml"), []byte("name: root_app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootRepo := filepath.Join(t.TempDir(), "root-repo")
	pkgRepo := filepath.Join(t.TempDir(), "core-repo")
	git := &fakeAddRepoGit{discovered: []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: rootRepo, PackageName: "root_app"},
		{Name: "core-pkg", RepoRoot: pkgRepo, PackageName: "core"},
	}}
	registry := &fakeAddRepoRegistry{records: []domain.RegistryRecord{{
		Name: "feature-login", Branch: "feature/login", Path: rootPath, RepoRoot: rootRepo, Status: "active",
	}}}

	service := NewAddRepoService(git, registry, &fakeAddRepoPrompt{})
	_, err := service.Run(domain.AddRepoInput{
		WorkspaceName:  "feature-login",
		ExecutionScope: ".",
		RepoSelectors:  []string{"core-pkg"},
		SyncPolicy:     domain.AddRepoSyncNever,
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("expected success, got=%v", err)
	}

	workspacePath := filepath.Join(containerPath, "feature-login.code-workspace")
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("expected add-repo to skip workspace generation when file is missing, stat err=%v", err)
	}
}

func TestResolveSyncPolicyAutoConfirmTrue(t *testing.T) {
	service := NewAddRepoService(&fakeAddRepoGit{}, &fakeAddRepoRegistry{}, &fakeAddRepoPrompt{confirmResponses: []bool{true}})

	sync, err := service.resolveSyncPolicy(domain.AddRepoInput{SyncPolicy: domain.AddRepoSyncAuto, NonInteractive: false})
	if err != nil {
		t.Fatalf("expected success, got=%v", err)
	}
	if !sync {
		t.Fatalf("expected sync=true when user confirms")
	}
}

func TestResolveSyncPolicyUnknownPolicyReturnsError(t *testing.T) {
	service := NewAddRepoService(&fakeAddRepoGit{}, &fakeAddRepoRegistry{}, &fakeAddRepoPrompt{})

	_, err := service.resolveSyncPolicy(domain.AddRepoInput{SyncPolicy: domain.AddRepoSyncPolicy("invalid")})
	if err == nil {
		t.Fatalf("expected unknown policy error")
	}
	if !strings.Contains(err.Error(), "Unknown add-repo sync policy") {
		t.Fatalf("expected unknown policy message, got=%v", err)
	}
}

func TestResolvePackageBranchingUsesRepoRootFallbackKeys(t *testing.T) {
	service := NewAddRepoService(&fakeAddRepoGit{}, &fakeAddRepoRegistry{}, &fakeAddRepoPrompt{})
	repo := domain.DiscoveredFlutterRepo{Name: "core-pkg", RepoRoot: "/tmp/core"}

	branch, base, err := service.resolvePackageBranching(domain.AddRepoInput{
		PackageBranchSource: map[string]string{"/tmp/core": "feature/core"},
		PackageBaseBranch:   map[string]string{"/tmp/core": "release/1.0"},
		NonInteractive:      true,
	}, repo, "feature/login", false)
	if err != nil {
		t.Fatalf("expected success, got=%v", err)
	}
	if branch != "feature/core" || base != "release/1.0" {
		t.Fatalf("expected repo-root fallback values, got branch=%q base=%q", branch, base)
	}
}

func TestWorkspacePackageRecordsFiltersAndSorts(t *testing.T) {
	records := []domain.RegistryRecord{
		{Name: "feature-login__pkg__zeta"},
		{Name: "feature-login"},
		{Name: "feature-login__pkg__alpha"},
		{Name: "other__pkg__core"},
	}

	got := workspacePackageRecords("feature-login", records)
	if len(got) != 2 {
		t.Fatalf("expected two package records, got=%d", len(got))
	}
	if got[0].Name != "feature-login__pkg__alpha" || got[1].Name != "feature-login__pkg__zeta" {
		t.Fatalf("expected sorted package records, got=%v", got)
	}
}

func TestReadPackageNameFromWorktreeParsesNameAndFallsBack(t *testing.T) {
	t.Run("parses_quoted_name", func(t *testing.T) {
		repoPath := t.TempDir()
		content := "# comment\nname: 'quoted_pkg'\n"
		if err := os.WriteFile(filepath.Join(repoPath, "pubspec.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		if got := readPackageNameFromWorktree(repoPath); got != "quoted_pkg" {
			t.Fatalf("expected parsed package name, got=%q", got)
		}
	})

	t.Run("falls_back_to_dir_name_when_missing", func(t *testing.T) {
		base := t.TempDir()
		repoPath := filepath.Join(base, "core-repo")
		if err := os.MkdirAll(repoPath, 0o755); err != nil {
			t.Fatal(err)
		}

		if got := readPackageNameFromWorktree(repoPath); got != "core-repo" {
			t.Fatalf("expected dir-name fallback, got=%q", got)
		}
	})
}
