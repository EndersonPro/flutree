package app

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakePub struct {
	mu sync.Mutex

	toolByPath map[string]domain.PubTool
	errByKey   map[string]error
	opsByPath  map[string][]string

	pkgPaths       map[string]struct{}
	pkgStartSignal chan string
	pkgRelease     chan struct{}
	rootStarted    bool
}

func (f *fakePub) DetectTool(repoPath string) (domain.PubTool, error) {
	f.record(repoPath, "detect")
	if err := f.errFor("detect", repoPath); err != nil {
		return "", err
	}
	if tool, ok := f.toolByPath[repoPath]; ok {
		return tool, nil
	}
	return domain.PubToolFlutter, nil
}

func (f *fakePub) Clean(repoPath string, tool domain.PubTool) error {
	f.record(repoPath, "clean")
	return f.errFor("clean", repoPath)
}

func (f *fakePub) RemoveLock(repoPath string) error {
	f.record(repoPath, "remove-lock")
	return f.errFor("remove-lock", repoPath)
}

func (f *fakePub) Get(repoPath string, tool domain.PubTool) error {
	f.record(repoPath, "get")
	if _, isPackage := f.pkgPaths[repoPath]; isPackage {
		if f.pkgStartSignal != nil {
			f.pkgStartSignal <- repoPath
		}
		if f.pkgRelease != nil {
			<-f.pkgRelease
		}
	} else {
		f.mu.Lock()
		f.rootStarted = true
		f.mu.Unlock()
	}
	return f.errFor("get", repoPath)
}

func (f *fakePub) errFor(step, repoPath string) error {
	if f.errByKey == nil {
		return nil
	}
	return f.errByKey[step+"::"+repoPath]
}

func (f *fakePub) record(repoPath, step string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.opsByPath == nil {
		f.opsByPath = map[string][]string{}
	}
	f.opsByPath[repoPath] = append(f.opsByPath[repoPath], step)
}

func TestPubGetRunsPackagesInParallelThenRoot(t *testing.T) {
	rootPath := "/tmp/worktrees/demo/root/root-app"
	pkgAPath := "/tmp/worktrees/demo/packages/a"
	pkgBPath := "/tmp/worktrees/demo/packages/b"

	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Path: rootPath, RepoRoot: "/tmp/repo-root", Status: "active"},
		{Name: "demo__pkg__a", Path: pkgAPath, RepoRoot: "/tmp/repo-a", Status: "active"},
		{Name: "demo__pkg__b", Path: pkgBPath, RepoRoot: "/tmp/repo-b", Status: "active"},
	}}

	p := &fakePub{
		pkgPaths:       map[string]struct{}{pkgAPath: {}, pkgBPath: {}},
		pkgStartSignal: make(chan string, 2),
		pkgRelease:     make(chan struct{}),
	}

	svc := NewPubGetService(r, p)

	resultCh := make(chan error, 1)
	go func() {
		_, err := svc.Run(domain.PubGetInput{Name: "demo"})
		resultCh <- err
	}()

	<-p.pkgStartSignal
	<-p.pkgStartSignal

	p.mu.Lock()
	rootStartedEarly := p.rootStarted
	p.mu.Unlock()
	if rootStartedEarly {
		t.Fatalf("expected root pub get to wait until package phase ends")
	}

	close(p.pkgRelease)
	if err := <-resultCh; err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	p.mu.Lock()
	rootStarted := p.rootStarted
	p.mu.Unlock()
	if !rootStarted {
		t.Fatalf("expected root pub get execution after package phase")
	}
}

func TestPubGetForceRunsCleanAndLockRemovalBeforeGet(t *testing.T) {
	rootPath := "/tmp/worktrees/demo/root/root-app"
	pkgPath := "/tmp/worktrees/demo/packages/core"

	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Path: rootPath, RepoRoot: "/tmp/repo-root", Status: "active"},
		{Name: "demo__pkg__core", Path: pkgPath, RepoRoot: "/tmp/repo-core", Status: "active"},
	}}
	p := &fakePub{}

	svc := NewPubGetService(r, p)
	if _, err := svc.Run(domain.PubGetInput{Name: "demo", Force: true}); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	want := []string{"detect", "clean", "remove-lock", "get"}
	if !reflect.DeepEqual(p.opsByPath[pkgPath], want) {
		t.Fatalf("unexpected package op order. got=%v want=%v", p.opsByPath[pkgPath], want)
	}
	if !reflect.DeepEqual(p.opsByPath[rootPath], want) {
		t.Fatalf("unexpected root op order. got=%v want=%v", p.opsByPath[rootPath], want)
	}
}

func TestPubGetStopsBeforeRootWhenPackageFails(t *testing.T) {
	rootPath := "/tmp/worktrees/demo/root/root-app"
	pkgPath := "/tmp/worktrees/demo/packages/core"

	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Path: rootPath, RepoRoot: "/tmp/repo-root", Status: "active"},
		{Name: "demo__pkg__core", Path: pkgPath, RepoRoot: "/tmp/repo-core", Status: "active"},
	}}
	p := &fakePub{
		pkgPaths: map[string]struct{}{pkgPath: {}},
		errByKey: map[string]error{
			"get::" + pkgPath: errors.New("network timeout"),
		},
	}

	svc := NewPubGetService(r, p)
	_, err := svc.Run(domain.PubGetInput{Name: "demo"})
	if err == nil {
		t.Fatalf("expected failure when package pub get fails")
	}
	if !strings.Contains(err.Error(), "Failed to run pub get for one or more package repositories") {
		t.Fatalf("unexpected error message: %v", err)
	}

	p.mu.Lock()
	rootStarted := p.rootStarted
	p.mu.Unlock()
	if rootStarted {
		t.Fatalf("expected root execution to be skipped when package phase fails")
	}
}

func TestPubGetPropagatesRootFailureAfterPackages(t *testing.T) {
	rootPath := "/tmp/worktrees/demo/root/root-app"
	pkgPath := "/tmp/worktrees/demo/packages/core"

	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Path: rootPath, RepoRoot: "/tmp/repo-root", Status: "active"},
		{Name: "demo__pkg__core", Path: pkgPath, RepoRoot: "/tmp/repo-core", Status: "active"},
	}}
	p := &fakePub{
		pkgPaths: map[string]struct{}{pkgPath: {}},
		errByKey: map[string]error{
			"get::" + rootPath: errors.New("sdk missing"),
		},
	}

	svc := NewPubGetService(r, p)
	_, err := svc.Run(domain.PubGetInput{Name: "demo"})
	if err == nil {
		t.Fatalf("expected failure when root pub get fails")
	}
	if !strings.Contains(err.Error(), "Failed to run pub get for root repository") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
