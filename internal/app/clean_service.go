package app

import (
	"path/filepath"

	"github.com/EndersonPro/flutree/internal/domain"
)

type CleanService struct {
	git      GitPort
	registry RegistryPort
	pub      PubPort
}

func NewCleanService(git GitPort, registry RegistryPort, pub PubPort) *CleanService {
	return &CleanService{git: git, registry: registry, pub: pub}
}

func (s *CleanService) Run(input domain.CleanInput) (domain.CleanResult, error) {
	currentRepo, err := s.git.EnsureRepo()
	if err != nil {
		return domain.CleanResult{}, err
	}
	currentRepo = filepath.Clean(currentRepo)

	records, err := s.registry.ListRecords()
	if err != nil {
		return domain.CleanResult{}, err
	}

	record, ok := findRecordByPath(records, currentRepo)
	if !ok {
		return domain.CleanResult{}, domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Current repository is not a managed flutree worktree.",
			"Run this command from a managed worktree shown by 'flutree list'.",
			nil,
		)
	}

	tool, err := s.pub.DetectTool(record.Path)
	if err != nil {
		return domain.CleanResult{}, err
	}
	if err := s.pub.Clean(record.Path, tool); err != nil {
		return domain.CleanResult{}, err
	}

	lockRemoved := false
	if input.Force {
		if err := s.pub.RemoveLock(record.Path); err != nil {
			return domain.CleanResult{}, err
		}
		lockRemoved = true
	}

	return domain.CleanResult{
		Record:      record,
		Tool:        tool,
		Force:       input.Force,
		LockRemoved: lockRemoved,
	}, nil
}

func findRecordByPath(records []domain.RegistryRecord, path string) (domain.RegistryRecord, bool) {
	cleanPath := filepath.Clean(path)
	for _, record := range records {
		if filepath.Clean(record.Path) == cleanPath {
			return record, true
		}
	}
	return domain.RegistryRecord{}, false
}
