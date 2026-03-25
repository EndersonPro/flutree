package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type Repository struct {
	Path string
}

func NewDefault() *Repository {
	home, _ := os.UserHomeDir()
	return &Repository{Path: filepath.Join(home, "Documents", "worktrees", ".worktrees_registry.json")}
}

func (r *Repository) ensureExists() error {
	if _, err := os.Stat(r.Path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to create registry directory.", r.Path, err)
	}
	def := domain.RegistryDocument{Version: 1, Records: []domain.RegistryRecord{}}
	return r.save(def)
}

func (r *Repository) load() (domain.RegistryDocument, error) {
	if err := r.ensureExists(); err != nil {
		return domain.RegistryDocument{}, err
	}
	b, err := os.ReadFile(r.Path)
	if err != nil {
		return domain.RegistryDocument{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to read registry file.", r.Path, err)
	}
	var doc domain.RegistryDocument
	if err := json.Unmarshal(b, &doc); err != nil {
		return domain.RegistryDocument{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to parse registry file.", r.Path, err)
	}
	if doc.Version == 0 {
		doc.Version = 1
	}
	if doc.Records == nil {
		doc.Records = []domain.RegistryRecord{}
	}
	if err := validate(doc); err != nil {
		return domain.RegistryDocument{}, err
	}
	return doc, nil
}

func (r *Repository) save(doc domain.RegistryDocument) error {
	if err := validate(doc); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to serialize registry file.", r.Path, err)
	}
	tmp := fmt.Sprintf("%s.tmp", r.Path)
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to write registry temp file.", tmp, err)
	}
	if err := os.Rename(tmp, r.Path); err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to atomically replace registry file.", r.Path, err)
	}
	return nil
}

func (r *Repository) ListRecords() ([]domain.RegistryRecord, error) {
	doc, err := r.load()
	if err != nil {
		return nil, err
	}
	return doc.Records, nil
}

func (r *Repository) Get(name string) (domain.RegistryRecord, error) {
	recs, err := r.ListRecords()
	if err != nil {
		return domain.RegistryRecord{}, err
	}
	for _, rec := range recs {
		if rec.Name == name {
			return rec, nil
		}
	}
	return domain.RegistryRecord{}, domain.NewError(domain.CategoryPrecondition, 3, fmt.Sprintf("Managed worktree '%s' was not found in registry.", name), "Run 'flutree list' to inspect managed entries.", nil)
}

func (r *Repository) Upsert(record domain.RegistryRecord) error {
	doc, err := r.load()
	if err != nil {
		return err
	}
	next := make([]domain.RegistryRecord, 0, len(doc.Records)+1)
	for _, rec := range doc.Records {
		if rec.Name == record.Name {
			continue
		}
		next = append(next, rec)
	}
	next = append(next, record)
	doc.Records = next
	return r.save(doc)
}

func (r *Repository) Remove(name string) (domain.RegistryRecord, error) {
	doc, err := r.load()
	if err != nil {
		return domain.RegistryRecord{}, err
	}
	found := false
	var removed domain.RegistryRecord
	next := make([]domain.RegistryRecord, 0, len(doc.Records))
	for _, rec := range doc.Records {
		if rec.Name == name {
			removed = rec
			found = true
			continue
		}
		next = append(next, rec)
	}
	if !found {
		return domain.RegistryRecord{}, domain.NewError(domain.CategoryPrecondition, 3, fmt.Sprintf("Managed worktree '%s' was not found in registry.", name), "Run 'flutree list' to inspect managed entries.", nil)
	}
	doc.Records = next
	return removed, r.save(doc)
}

func (r *Repository) MarkCompleted(name string) (domain.RegistryRecord, error) {
	return r.Remove(name)
}

func validate(doc domain.RegistryDocument) error {
	byName := map[string]struct{}{}
	byBranch := map[string]struct{}{}
	byPath := map[string]struct{}{}
	for _, rec := range doc.Records {
		if _, ok := byName[rec.Name]; ok {
			return domain.NewError(domain.CategoryPersistence, 5, fmt.Sprintf("Registry integrity failed: duplicate name '%s'.", rec.Name), "Fix duplicates in registry file.", nil)
		}
		byName[rec.Name] = struct{}{}

		bk := fmt.Sprintf("%s::%s", rec.RepoRoot, rec.Branch)
		if _, ok := byBranch[bk]; ok {
			return domain.NewError(domain.CategoryPersistence, 5, fmt.Sprintf("Registry integrity failed: duplicate branch '%s' for repo.", rec.Branch), "Fix duplicates in registry file.", nil)
		}
		byBranch[bk] = struct{}{}

		path := strings.TrimSpace(rec.Path)
		if _, ok := byPath[path]; ok {
			return domain.NewError(domain.CategoryPersistence, 5, fmt.Sprintf("Registry integrity failed: duplicate path '%s'.", rec.Path), "Fix duplicates in registry file.", nil)
		}
		byPath[path] = struct{}{}
	}
	sort.Slice(doc.Records, func(i, j int) bool { return doc.Records[i].Name < doc.Records[j].Name })
	return nil
}
