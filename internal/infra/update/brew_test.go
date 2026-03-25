package update

import "testing"

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
