package update

import (
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

func TestBrewGatewayReturnsWindowsHint(t *testing.T) {
	gw := &BrewGateway{goos: "windows"}
	err := gw.CheckBrewInstalled()
	if err == nil {
		t.Fatal("expected precondition error on windows")
	}
	ae, ok := err.(*domain.AppError)
	if !ok {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	combined := ae.Message + " " + ae.Hint
	if !strings.Contains(combined, "scoop update flutree") {
		t.Fatalf("expected scoop hint in error, got message=%q hint=%q", ae.Message, ae.Hint)
	}
}

func TestParseBrewOutdatedJSONParsesInstalledAndLatest(t *testing.T) {
	input := `{"formulae":[{"name":"flutree","installed_versions":["0.7.0"],"current_version":"0.8.0"}]}`
	current, latest, ok := parseBrewOutdatedJSON(input)
	if !ok {
		t.Fatalf("expected parser success")
	}
	if current != "0.7.0" {
		t.Fatalf("unexpected current version: %s", current)
	}
	if latest != "0.8.0" {
		t.Fatalf("unexpected latest version: %s", latest)
	}
}

func TestParseBrewOutdatedJSONRejectsInvalidPayload(t *testing.T) {
	_, _, ok := parseBrewOutdatedJSON(`{"formulae":[]}`)
	if ok {
		t.Fatalf("expected parser failure for empty formulae")
	}
}
