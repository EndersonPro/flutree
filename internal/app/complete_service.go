package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type CompleteService struct {
	git      GitPort
	registry RegistryPort
	prompt   PromptPort
}

func NewCompleteService(git GitPort, registry RegistryPort, prompt PromptPort) *CompleteService {
	return &CompleteService{git: git, registry: registry, prompt: prompt}
}

func (s *CompleteService) Run(input domain.CompleteInput) (domain.CompleteResult, error) {
	rec, err := s.registry.Get(input.Name)
	if err != nil {
		return domain.CompleteResult{}, err
	}

	containerPath, removeContainer, err := completionContainerPath(rec)
	if err != nil {
		return domain.CompleteResult{}, err
	}

	records, err := s.registry.ListRecords()
	if err != nil {
		return domain.CompleteResult{}, err
	}

	targets := associatedCompletionTargets(rec, records)

	for _, target := range targets {
		dirty, err := s.git.IsDirty(target.Path)
		if err != nil {
			return domain.CompleteResult{}, err
		}
		if dirty && !input.Force {
			return domain.CompleteResult{}, domain.NewError(
				domain.CategoryPrecondition, 3,
				"Worktree '"+target.Name+"' has uncommitted changes.",
				"Commit/stash changes, or use --force if you intentionally want removal.",
				nil,
			)
		}
	}

	confirmMessage := "Remove worktree '" + rec.Name + "' at '" + rec.Path + "'?"
	if len(targets) > 1 {
		confirmMessage = "Remove worktree '" + rec.Name + "' and its associated package worktrees?"
	}

	confirmed, err := s.prompt.Confirm(
		confirmMessage,
		input.NonInteractive,
		input.Yes,
	)
	if err != nil {
		return domain.CompleteResult{}, err
	}
	if !confirmed {
		return domain.CompleteResult{}, domain.NewError(domain.CategoryInput, 2, "Completion cancelled by user.", "", nil)
	}

	for _, target := range targets {
		if err := s.git.RemoveWorktree(target.RepoRoot, target.Path, input.Force); err != nil {
			return domain.CompleteResult{}, err
		}
	}

	if removeContainer {
		if err := os.RemoveAll(containerPath); err != nil {
			return domain.CompleteResult{}, domain.NewError(
				domain.CategoryPersistence,
				5,
				"Failed to remove worktree container directory.",
				containerPath,
				err,
			)
		}
	}

	for _, target := range targets {
		if _, err := s.registry.MarkCompleted(target.Name); err != nil {
			return domain.CompleteResult{}, err
		}
	}

	return domain.CompleteResult{
		Record:        rec,
		RemovedBranch: false,
		NextStep:      "",
	}, nil
}

func associatedCompletionTargets(record domain.RegistryRecord, records []domain.RegistryRecord) []domain.RegistryRecord {
	if _, isPackage := splitPackageRecordName(record.Name); isPackage {
		return []domain.RegistryRecord{record}
	}

	targets := []domain.RegistryRecord{record}
	prefix := record.Name + "__pkg__"
	for _, candidate := range records {
		if len(candidate.Name) > len(prefix) && candidate.Name[:len(prefix)] == prefix {
			targets = append(targets, candidate)
		}
	}
	return targets
}

func completionContainerPath(record domain.RegistryRecord) (string, bool, error) {
	if _, isPackage := splitPackageRecordName(record.Name); isPackage {
		return "", false, nil
	}

	recordPath := filepath.Clean(record.Path)
	rootSegment := filepath.Dir(recordPath)
	if filepath.Base(rootSegment) != "root" {
		return "", false, domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Cannot determine worktree container for root completion.",
			"Expected root worktree path in '<container>/root/<repository>'.",
			nil,
		)
	}

	container := filepath.Clean(filepath.Dir(rootSegment))
	if container == "" || container == "." || container == string(filepath.Separator) {
		return "", false, domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Cannot determine safe container path for completion.",
			"Container path resolved to a protected location.",
			nil,
		)
	}

	scope := filepath.Clean(destinationRoot())
	if !isPathWithinScope(container, scope) {
		return "", false, domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Refusing to remove container outside managed destination root.",
			"Container: "+container+" | Expected root: "+scope,
			nil,
		)
	}

	if filepath.Base(container) != normalizeWorktreeName(record.Name) {
		return "", false, domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Refusing to remove container with mismatched worktree name.",
			"Container folder: "+filepath.Base(container)+" | Record name: "+normalizeWorktreeName(record.Name),
			nil,
		)
	}

	return container, true, nil
}

func isPathWithinScope(path, scope string) bool {
	if path == "" || scope == "" {
		return false
	}
	rel, err := filepath.Rel(scope, path)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
