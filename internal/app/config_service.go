package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

const (
	scopeRootKey         = "scope.root"
	workspaceGenerateKey = "workspace.generate"
	sdkManagerKey        = "sdk.manager"
)

const supportedKeysHint = "Supported keys: scope.root, workspace.generate, sdk.manager."

type ConfigService struct {
	config ConfigPort
}

func NewConfigService(config ConfigPort) *ConfigService {
	return &ConfigService{config: config}
}

// Set stores a configuration value and returns the normalized stored value.
func (s *ConfigService) Set(key, value string) (string, error) {
	switch strings.TrimSpace(key) {
	case scopeRootKey:
		return s.SetScopeRoot(key, value)
	case workspaceGenerateKey:
		enabled, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return "", domain.NewError(domain.CategoryInput, 2, "Invalid value for workspace.generate.", "Use true or false.", err)
		}
		doc, err := s.config.Load()
		if err != nil {
			return "", err
		}
		if doc.Version == 0 {
			doc.Version = 1
		}
		doc.Workspace.Generate = &enabled
		if err := s.config.Save(doc); err != nil {
			return "", err
		}
		return strconv.FormatBool(enabled), nil
	case sdkManagerKey:
		manager, err := parseSDKManager(value)
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
		doc.SDK.Manager = manager
		if err := s.config.Save(doc); err != nil {
			return "", err
		}
		return string(manager), nil
	default:
		return "", unsupportedKeyError(key)
	}
}

// Get returns the effective value of a configuration key.
func (s *ConfigService) Get(key string) (string, error) {
	switch strings.TrimSpace(key) {
	case scopeRootKey:
		return s.GetScopeRoot(key)
	case workspaceGenerateKey:
		enabled, err := s.GenerateWorkspace()
		if err != nil {
			return "", err
		}
		return strconv.FormatBool(enabled), nil
	case sdkManagerKey:
		manager, err := s.SDKManager()
		if err != nil {
			return "", err
		}
		return string(manager), nil
	default:
		return "", unsupportedKeyError(key)
	}
}

// SDKManager returns the preferred SDK manager for new worktrees (default auto).
func (s *ConfigService) SDKManager() (domain.SDKManager, error) {
	doc, err := s.config.Load()
	if err != nil {
		return "", err
	}
	if doc.SDK.Manager == "" {
		return domain.SDKManagerAuto, nil
	}
	return doc.SDK.Manager, nil
}

func parseSDKManager(value string) (domain.SDKManager, error) {
	switch m := domain.SDKManager(strings.ToLower(strings.TrimSpace(value))); m {
	case domain.SDKManagerAuto, domain.SDKManagerFVM, domain.SDKManagerPuro, domain.SDKManagerNone:
		return m, nil
	default:
		return "", domain.NewError(domain.CategoryInput, 2, "Invalid value for sdk.manager.", "Use auto, fvm, puro or none.", nil)
	}
}

// GenerateWorkspace reports whether create should write a VSCode workspace
// when neither --workspace nor --no-workspace is passed. Defaults to true.
func (s *ConfigService) GenerateWorkspace() (bool, error) {
	doc, err := s.config.Load()
	if err != nil {
		return false, err
	}
	if doc.Workspace.Generate == nil {
		return true, nil
	}
	return *doc.Workspace.Generate, nil
}

func unsupportedKeyError(key string) error {
	return domain.NewError(domain.CategoryInput, 2, "Unsupported config key '"+strings.TrimSpace(key)+"'.", supportedKeysHint, nil)
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
	return unsupportedKeyError(trimmed)
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
