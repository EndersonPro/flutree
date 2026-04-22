package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeCleanPub struct {
	toolByPath map[string]domain.PubTool
	errByStep  map[string]error
	opsByPath  map[string][]string
}

func (f *fakeCleanPub) DetectTool(repoPath string) (domain.PubTool, error) {
	f.record(repoPath, "detect")
	if err := f.stepErr("detect"); err != nil {
		return "", err
	}
	if tool, ok := f.toolByPath[repoPath]; ok {
		return tool, nil
	}
	return domain.PubToolFlutter, nil
}

func (f *fakeCleanPub) Clean(repoPath string, tool domain.PubTool) error {
	f.record(repoPath, "clean")
	return f.stepErr("clean")
}

func (f *fakeCleanPub) RemoveLock(repoPath string) error {
	f.record(repoPath, "remove-lock")
	return f.stepErr("remove-lock")
}

func (f *fakeCleanPub) Get(repoPath string, tool domain.PubTool) error { return nil }

func (f *fakeCleanPub) record(repoPath, step string) {
	if f.opsByPath == nil {
		f.opsByPath = map[string][]string{}
	}
	f.opsByPath[repoPath] = append(f.opsByPath[repoPath], step)
}

func (f *fakeCleanPub) stepErr(step string) error {
	if f.errByStep == nil {
		return nil
	}
	return f.errByStep[step]
}

func TestCleanRunsOnCurrentManagedWorktree(t *testing.T) {
	repoPath := "/tmp/worktrees/demo/root/root-app"
	g := &fakeGit{currentRepo: repoPath}
	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Path: repoPath, RepoRoot: "/tmp/repo-root", Status: "active"},
	}}
	p := &fakeCleanPub{}

	svc := NewCleanService(g, r, p)
	result, err := svc.Run(domain.CleanInput{})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result.Record.Name != "demo" || result.Tool != domain.PubToolFlutter {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(p.opsByPath[repoPath], []string{"detect", "clean"}) {
		t.Fatalf("unexpected operations: %v", p.opsByPath[repoPath])
	}
}

func TestCleanForceRemovesLockAfterClean(t *testing.T) {
	repoPath := "/tmp/worktrees/demo/root/root-app"
	g := &fakeGit{currentRepo: repoPath}
	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Path: repoPath, RepoRoot: "/tmp/repo-root", Status: "active"},
	}}
	p := &fakeCleanPub{}

	svc := NewCleanService(g, r, p)
	result, err := svc.Run(domain.CleanInput{Force: true})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !result.LockRemoved || !result.Force {
		t.Fatalf("expected force+lock removal metadata, got %+v", result)
	}
	if !reflect.DeepEqual(p.opsByPath[repoPath], []string{"detect", "clean", "remove-lock"}) {
		t.Fatalf("unexpected operations: %v", p.opsByPath[repoPath])
	}
}

func TestCleanFailsOutsideManagedWorktree(t *testing.T) {
	g := &fakeGit{currentRepo: "/tmp/unmanaged/repo"}
	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Path: "/tmp/worktrees/demo/root/root-app", RepoRoot: "/tmp/repo-root", Status: "active"},
	}}
	p := &fakeCleanPub{}

	svc := NewCleanService(g, r, p)
	_, err := svc.Run(domain.CleanInput{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "Current repository is not a managed flutree worktree." {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanStopsOnCleanError(t *testing.T) {
	repoPath := "/tmp/worktrees/demo/root/root-app"
	g := &fakeGit{currentRepo: repoPath}
	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Path: repoPath, RepoRoot: "/tmp/repo-root", Status: "active"},
	}}
	p := &fakeCleanPub{errByStep: map[string]error{"clean": errors.New("flutter not found")}}

	svc := NewCleanService(g, r, p)
	_, err := svc.Run(domain.CleanInput{Force: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "flutter not found" {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(p.opsByPath[repoPath], []string{"detect", "clean"}) {
		t.Fatalf("expected lock removal to be skipped, got %v", p.opsByPath[repoPath])
	}
}
