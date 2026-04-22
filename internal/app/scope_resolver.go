package app

import (
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type ScopeResolver struct {
	config ConfigPort
}

func NewScopeResolver(config ConfigPort) *ScopeResolver {
	return &ScopeResolver{config: config}
}

func (r *ScopeResolver) Resolve(scopeFlag string, scopeFlagProvided bool) (string, error) {
	if scopeFlagProvided {
		return normalizeAndValidateScopePath(scopeFlag, domain.CategoryInput, "--scope")
	}

	doc, err := r.config.Load()
	if err != nil {
		return "", err
	}
	if persisted := strings.TrimSpace(doc.Scope.Root); persisted != "" {
		resolved, resolveErr := normalizeAndValidateScopePath(persisted, domain.CategoryPrecondition, "persisted scope.root")
		if resolveErr == nil {
			return resolved, nil
		}
		return "", domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Persisted scope.root is invalid for discovery.",
			"Run `flutree config set scope.root <path>` with a valid reachable directory.",
			resolveErr,
		)
	}

	return normalizeAndValidateScopePath(".", domain.CategoryPrecondition, "default scope")
}
