package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/EndersonPro/flutree/internal/domain"
)

const supportedVersion = 1

type Repository struct {
	Path string
}

func NewDefault() *Repository {
	home, _ := os.UserHomeDir()
	return &Repository{Path: filepath.Join(home, "Documents", "worktrees", ".flutree_config.json")}
}

func (r *Repository) Load() (domain.UserConfigDocument, error) {
	if err := r.ensureExists(); err != nil {
		return domain.UserConfigDocument{}, err
	}

	b, err := os.ReadFile(r.Path)
	if err != nil {
		return domain.UserConfigDocument{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to read config file.", r.Path, err)
	}

	var doc domain.UserConfigDocument
	if err := json.Unmarshal(b, &doc); err != nil {
		return domain.UserConfigDocument{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to parse config file.", r.Path, err)
	}

	if doc.Version == 0 {
		doc.Version = supportedVersion
	}
	if doc.Version != supportedVersion {
		return domain.UserConfigDocument{}, domain.NewError(
			domain.CategoryPersistence,
			5,
			fmt.Sprintf("Unsupported config version '%d'.", doc.Version),
			fmt.Sprintf("Supported version: %d.", supportedVersion),
			nil,
		)
	}

	return doc, nil
}

func (r *Repository) Save(doc domain.UserConfigDocument) error {
	if doc.Version == 0 {
		doc.Version = supportedVersion
	}
	if doc.Version != supportedVersion {
		return domain.NewError(
			domain.CategoryPersistence,
			5,
			fmt.Sprintf("Unsupported config version '%d'.", doc.Version),
			fmt.Sprintf("Supported version: %d.", supportedVersion),
			nil,
		)
	}

	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to create config directory.", r.Path, err)
	}

	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to serialize config file.", r.Path, err)
	}

	tmp := r.Path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to write config temp file.", tmp, err)
	}
	if err := os.Rename(tmp, r.Path); err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to atomically replace config file.", r.Path, err)
	}

	return nil
}

func (r *Repository) ensureExists() error {
	if _, err := os.Stat(r.Path); err == nil {
		return nil
	}
	return r.Save(domain.UserConfigDocument{Version: supportedVersion})
}
