package update

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type BrewGateway struct{}

func (g *BrewGateway) CheckBrewInstalled() error {
	if _, err := exec.LookPath("brew"); err != nil {
		return domain.NewError(domain.CategoryPrecondition, 1, "Homebrew is required for automatic updates.", "Only brew-managed installations are supported for update in this release.", err)
	}
	return nil
}

func (g *BrewGateway) CheckOutdated(packageName string) (bool, string, string, error) {
	out, err := g.run("outdated", "--json=v2", packageName)
	if err != nil {
		return false, "", "", err
	}

	jsonText := strings.TrimSpace(out)
	if jsonText == "" || jsonText == "{}" {
		version, vErr := g.currentVersion(packageName)
		if vErr != nil {
			return false, "", "", vErr
		}
		return false, version, version, nil
	}

	current, latest, ok := parseBrewOutdatedJSON(jsonText)
	if !ok {
		return false, "", "", domain.NewError(domain.CategoryUnexpected, 1, "Failed to parse brew outdated output.", "Re-run with 'brew outdated --json=v2 flutree' to inspect raw output.", nil)
	}
	return true, current, latest, nil
}

func (g *BrewGateway) Upgrade(packageName string) (string, error) {
	if _, err := g.run("update"); err != nil {
		return "", err
	}
	out, err := g.run("upgrade", packageName)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g *BrewGateway) currentVersion(packageName string) (string, error) {
	out, err := g.run("list", "--versions", packageName)
	if err != nil {
		return "", err
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) < 2 {
		return "", domain.NewError(domain.CategoryUnexpected, 1, "Unable to detect installed flutree version from brew.", "Run 'brew list --versions flutree' manually and verify output.", nil)
	}
	return parts[1], nil
}

func (g *BrewGateway) run(args ...string) (string, error) {
	cmd := exec.Command("brew", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	details := strings.TrimSpace(string(out))
	return "", domain.NewError(domain.CategoryUnexpected, 1, fmt.Sprintf("Brew command failed: brew %s", strings.Join(args, " ")), details, err)
}

func parseBrewOutdatedJSON(input string) (current string, latest string, ok bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return "", "", false
	}
	formulae, ok := payload["formulae"].([]any)
	if !ok || len(formulae) == 0 {
		return "", "", false
	}
	entry, ok := formulae[0].(map[string]any)
	if !ok {
		return "", "", false
	}

	if value, ok := entry["current_version"].(string); ok {
		latest = value
	}
	if latest == "" {
		if currentInfo, ok := entry["current_version"].(map[string]any); ok {
			if value, ok := currentInfo["version"].(string); ok {
				latest = value
			}
		}
	}
	if installed, ok := entry["installed_versions"].([]any); ok && len(installed) > 0 {
		if value, ok := installed[0].(string); ok {
			current = value
		}
	}
	if current == "" {
		if value, ok := entry["installed_version"].(string); ok {
			current = value
		}
	}

	current = strings.TrimSpace(current)
	latest = strings.TrimSpace(latest)
	if current == "" || latest == "" {
		return "", "", false
	}
	return current, latest, true
}
