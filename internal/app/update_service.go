package app

import (
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

const brewPackageName = "flutree"

type UpdateService struct {
	updater UpdatePort
}

func NewUpdateService(updater UpdatePort) *UpdateService {
	return &UpdateService{updater: updater}
}

func (s *UpdateService) Run(input domain.UpdateInput) (domain.UpdateResult, error) {
	if input.Check && input.Apply {
		return domain.UpdateResult{}, domain.NewError(domain.CategoryInput, 2, "Use either --check or --apply, not both.", "Run 'flutree update --check' or 'flutree update --apply'.", nil)
	}

	apply := input.Apply || !input.Check
	if err := s.updater.CheckBrewInstalled(); err != nil {
		return domain.UpdateResult{}, err
	}

	outdated, current, latest, err := s.updater.CheckOutdated(brewPackageName)
	if err != nil {
		return domain.UpdateResult{}, err
	}

	if !apply {
		return domain.UpdateResult{
			Mode:     "check",
			Outdated: outdated,
			Current:  strings.TrimSpace(current),
			Latest:   strings.TrimSpace(latest),
		}, nil
	}

	notes := ""
	if outdated {
		notes, err = s.updater.Upgrade(brewPackageName)
		if err != nil {
			return domain.UpdateResult{}, err
		}
	}

	return domain.UpdateResult{
		Mode:         "apply",
		Outdated:     outdated,
		Current:      strings.TrimSpace(current),
		Latest:       strings.TrimSpace(latest),
		UpgradeNotes: strings.TrimSpace(notes),
	}, nil
}
