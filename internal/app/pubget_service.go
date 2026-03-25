package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/EndersonPro/flutree/internal/domain"
)

type PubGetService struct {
	registry RegistryPort
	pub      PubPort
}

func NewPubGetService(registry RegistryPort, pub PubPort) *PubGetService {
	return &PubGetService{registry: registry, pub: pub}
}

func (s *PubGetService) Run(input domain.PubGetInput) (domain.PubGetResult, error) {
	workspaceName := strings.TrimSpace(input.Name)
	if workspaceName == "" {
		return domain.PubGetResult{}, domain.NewError(
			domain.CategoryInput,
			2,
			"Missing workspace name.",
			"Usage: flutree pubget <name> [--force]",
			nil,
		)
	}

	records, err := s.registry.ListRecords()
	if err != nil {
		return domain.PubGetResult{}, err
	}

	rootRecord, packageRecords, err := resolveWorkspaceRecords(workspaceName, records)
	if err != nil {
		return domain.PubGetResult{}, err
	}

	packages, packageFailures := s.runPackages(packageRecords, input.Force)
	if len(packageFailures) > 0 {
		return domain.PubGetResult{}, domain.NewError(
			domain.CategoryUnexpected,
			1,
			"Failed to run pub get for one or more package repositories.",
			strings.Join(packageFailures, "\n"),
			nil,
		)
	}

	root, err := s.runRepo(rootRecord, "root", input.Force)
	if err != nil {
		return domain.PubGetResult{}, domain.NewError(
			domain.CategoryUnexpected,
			1,
			"Failed to run pub get for root repository.",
			err.Error(),
			nil,
		)
	}

	return domain.PubGetResult{
		WorkspaceName: rootRecord.Name,
		Root:          root,
		Packages:      packages,
		Force:         input.Force,
	}, nil
}

func (s *PubGetService) runPackages(records []domain.RegistryRecord, force bool) ([]domain.PubGetRepoResult, []string) {
	if len(records) == 0 {
		return []domain.PubGetRepoResult{}, nil
	}

	results := make([]domain.PubGetRepoResult, len(records))
	failures := []string{}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, record := range records {
		i := i
		record := record
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := s.runRepo(record, "package", force)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, err.Error())
				return
			}
			results[i] = result
		}()
	}

	wg.Wait()

	if len(failures) > 1 {
		sort.Strings(failures)
	}

	if len(failures) > 0 {
		return nil, failures
	}

	return results, nil
}

func (s *PubGetService) runRepo(record domain.RegistryRecord, role string, force bool) (domain.PubGetRepoResult, error) {
	tool, err := s.pub.DetectTool(record.Path)
	if err != nil {
		return domain.PubGetRepoResult{}, fmt.Errorf("[%s] %s", repoLabelForError(role, record), err.Error())
	}

	if force {
		if err := s.pub.Clean(record.Path, tool); err != nil {
			return domain.PubGetRepoResult{}, fmt.Errorf("[%s] clean failed: %s", repoLabelForError(role, record), err.Error())
		}
		if err := s.pub.RemoveLock(record.Path); err != nil {
			return domain.PubGetRepoResult{}, fmt.Errorf("[%s] lock removal failed: %s", repoLabelForError(role, record), err.Error())
		}
	}

	if err := s.pub.Get(record.Path, tool); err != nil {
		return domain.PubGetRepoResult{}, fmt.Errorf("[%s] pub get failed: %s", repoLabelForError(role, record), err.Error())
	}

	return domain.PubGetRepoResult{
		Name: record.Name,
		Path: record.Path,
		Tool: tool,
		Role: role,
	}, nil
}

func resolveWorkspaceRecords(name string, records []domain.RegistryRecord) (domain.RegistryRecord, []domain.RegistryRecord, error) {
	lookupName := strings.TrimSpace(name)
	if root, isPackage := splitPackageRecordName(lookupName); isPackage {
		lookupName = root
	}

	rootRecord, ok := findRecordByName(records, lookupName)
	if !ok {
		return domain.RegistryRecord{}, nil, domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Managed workspace '"+lookupName+"' was not found in registry.",
			"Run 'flutree list' to inspect managed entries.",
			nil,
		)
	}

	if _, isPackage := splitPackageRecordName(rootRecord.Name); isPackage {
		return domain.RegistryRecord{}, nil, domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Pub get requires a root managed workspace name.",
			"Pass the root workspace name shown by 'flutree list'.",
			nil,
		)
	}

	packages := []domain.RegistryRecord{}
	prefix := rootRecord.Name + "__pkg__"
	for _, candidate := range records {
		if strings.HasPrefix(candidate.Name, prefix) {
			packages = append(packages, candidate)
		}
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })

	return rootRecord, packages, nil
}

func findRecordByName(records []domain.RegistryRecord, name string) (domain.RegistryRecord, bool) {
	for _, rec := range records {
		if rec.Name == name {
			return rec, true
		}
	}
	return domain.RegistryRecord{}, false
}

func repoLabelForError(role string, record domain.RegistryRecord) string {
	return fmt.Sprintf("%s '%s' (%s)", role, record.Name, record.Path)
}
