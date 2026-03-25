package app

import (
	"path/filepath"
	"sort"

	"github.com/EndersonPro/flutree/internal/domain"
)

type ListService struct {
	git      GitPort
	registry RegistryPort
}

func NewListService(git GitPort, registry RegistryPort) *ListService {
	return &ListService{git: git, registry: registry}
}

func (s *ListService) Run(showAll bool) ([]domain.ListRow, error) {
	currentRepo, err := s.git.EnsureRepo()
	if err != nil {
		currentRepo = ""
	}

	allRecords, err := s.registry.ListRecords()
	if err != nil {
		return nil, err
	}

	records := make([]domain.RegistryRecord, 0, len(allRecords))
	if currentRepo == "" {
		records = allRecords
	} else {
		currentRepo = filepath.Clean(currentRepo)
		for _, rec := range allRecords {
			if filepath.Clean(rec.RepoRoot) == currentRepo {
				records = append(records, rec)
			}
		}
	}

	worktreesByRepo := map[string][]domain.GitWorktreeEntry{}
	for _, rec := range records {
		root := filepath.Clean(rec.RepoRoot)
		if _, ok := worktreesByRepo[root]; ok {
			continue
		}
		entries, err := s.git.ListWorktrees(root)
		if err != nil {
			continue
		}
		worktreesByRepo[root] = entries
	}

	branchByPath := map[string]string{}
	repoByPath := map[string]string{}
	for root, entries := range worktreesByRepo {
		for _, e := range entries {
			p := filepath.Clean(e.Path)
			branchByPath[p] = e.Branch
			repoByPath[p] = root
		}
	}

	managed := map[string]struct{}{}
	rows := make([]domain.ListRow, 0, len(records))
	for _, rec := range records {
		rp := filepath.Clean(rec.Path)
		managed[rp] = struct{}{}
		status := "missing"
		if rec.Status == "completed" {
			status = "completed"
		} else if _, ok := branchByPath[rp]; ok {
			status = "active"
		}
		rows = append(rows, domain.ListRow{
			Name:     rec.Name,
			Branch:   rec.Branch,
			Path:     rec.Path,
			RepoRoot: rec.RepoRoot,
			Status:   status,
		})
	}

	if showAll {
		for _, entries := range worktreesByRepo {
			for _, e := range entries {
				p := filepath.Clean(e.Path)
				if _, ok := managed[p]; ok {
					continue
				}
				branch := e.Branch
				if branch == "" {
					branch = "(detached)"
				}
				rows = append(rows, domain.ListRow{
					Name:     "-",
					Branch:   branch,
					Path:     e.Path,
					RepoRoot: repoByPath[p],
					Status:   "unmanaged",
				})
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Status != rows[j].Status {
			return rows[i].Status < rows[j].Status
		}
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].Path < rows[j].Path
	})
	return rows, nil
}
