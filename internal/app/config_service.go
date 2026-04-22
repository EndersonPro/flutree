package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

const scopeRootKey = "scope.root"

type ConfigService struct {
	config ConfigPort
}

func NewConfigService(config ConfigPort) *ConfigService {
	return &ConfigService{config: config}
}

func (s *ConfigService) SetScopeRoot(key, value string) (string, error) {
	if err := validateScopeRootKey(key); err != nil {
		return "", err
	}

	normalized, err := normalizeAndValidateScopePath(value, domain.CategoryInput, "scope.root")
	if err != nil {
		return "", err
	}

	doc, err := s.config.Load()
	if err != nil {
		return "", err
	}
	if doc.Version == 0 {
		doc.Version = 1
	}
	doc.Scope.Root = normalized

	if err := s.config.Save(doc); err != nil {
		return "", err
	}

	return normalized, nil
}

func (s *ConfigService) GetScopeRoot(key string) (string, error) {
	if err := validateScopeRootKey(key); err != nil {
		return "", err
	}

	doc, err := s.config.Load()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(doc.Scope.Root), nil
}

func validateScopeRootKey(key string) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == scopeRootKey {
		return nil
	}
	return domain.NewError(
		domain.CategoryInput,
		2,
		"Unsupported config key '"+trimmed+"'.",
		"Only 'scope.root' is currently supported.",
		nil,
	)
}

func normalizeAndValidateScopePath(raw string, category domain.ErrorCategory, source string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", domain.NewError(category, 2, "Scope root path is required.", "Provide a non-empty path for "+source+".", nil)
	}

	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", domain.NewError(category, 2, "Scope root path is invalid.", "Failed to normalize path for "+source+".", err)
	}
	abs = filepath.Clean(abs)

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", domain.NewError(category, 2, "Scope root path does not exist.", abs, nil)
		}
		return "", domain.NewError(category, 2, "Scope root path is invalid.", abs, err)
	}
	if !info.IsDir() {
		return "", domain.NewError(category, 2, "Scope root path must be a directory.", abs, nil)
	}

	if _, err := os.ReadDir(abs); err != nil {
		return "", domain.NewError(category, 2, "Scope root path is not reachable.", abs, err)
	}

	return abs, nil
}
