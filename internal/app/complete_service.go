package app

import "github.com/EndersonPro/flutree/internal/domain"

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

	dirty, err := s.git.IsDirty(rec.Path)
	if err != nil {
		return domain.CompleteResult{}, err
	}
	if dirty && !input.Force {
		return domain.CompleteResult{}, domain.NewError(
			domain.CategoryPrecondition, 3,
			"Worktree '"+rec.Name+"' has uncommitted changes.",
			"Commit/stash changes, or use --force if you intentionally want removal.",
			nil,
		)
	}

	confirmed, err := s.prompt.Confirm(
		"Remove worktree '"+rec.Name+"' at '"+rec.Path+"'?",
		input.NonInteractive,
		input.Yes,
	)
	if err != nil {
		return domain.CompleteResult{}, err
	}
	if !confirmed {
		return domain.CompleteResult{}, domain.NewError(domain.CategoryInput, 2, "Completion cancelled by user.", "", nil)
	}

	if err := s.git.RemoveWorktree(rec.RepoRoot, rec.Path, input.Force); err != nil {
		return domain.CompleteResult{}, err
	}
	if _, err := s.registry.MarkCompleted(rec.Name); err != nil {
		return domain.CompleteResult{}, err
	}

	return domain.CompleteResult{
		Record:        rec,
		RemovedBranch: false,
		NextStep:      "",
	}, nil
}
