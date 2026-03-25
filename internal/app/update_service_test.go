package app

import (
	"errors"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeUpdater struct {
	outdated       bool
	current        string
	latest         string
	upgradeNotes   string
	installErr     error
	checkErr       error
	upgradeErr     error
	upgradeInvoked bool
}

func (f *fakeUpdater) CheckBrewInstalled() error { return f.installErr }

func (f *fakeUpdater) CheckOutdated(string) (bool, string, string, error) {
	if f.checkErr != nil {
		return false, "", "", f.checkErr
	}
	return f.outdated, f.current, f.latest, nil
}

func (f *fakeUpdater) Upgrade(string) (string, error) {
	f.upgradeInvoked = true
	if f.upgradeErr != nil {
		return "", f.upgradeErr
	}
	return f.upgradeNotes, nil
}

func TestUpdateServiceCheckModeDoesNotUpgrade(t *testing.T) {
	updater := &fakeUpdater{outdated: true, current: "1.0.0", latest: "1.1.0"}
	svc := NewUpdateService(updater)

	result, err := svc.Run(domain.UpdateInput{Check: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Mode != "check" || !result.Outdated {
		t.Fatalf("unexpected check result: %+v", result)
	}
	if updater.upgradeInvoked {
		t.Fatalf("upgrade should not be called in check mode")
	}
}

func TestUpdateServiceApplyModeRunsUpgradeWhenOutdated(t *testing.T) {
	updater := &fakeUpdater{outdated: true, current: "1.0.0", latest: "1.1.0", upgradeNotes: "done"}
	svc := NewUpdateService(updater)

	result, err := svc.Run(domain.UpdateInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Mode != "apply" || !updater.upgradeInvoked {
		t.Fatalf("expected apply with upgrade call, got result=%+v called=%t", result, updater.upgradeInvoked)
	}
}

func TestUpdateServiceRejectsConflictingFlags(t *testing.T) {
	updater := &fakeUpdater{}
	svc := NewUpdateService(updater)

	_, err := svc.Run(domain.UpdateInput{Check: true, Apply: true})
	if err == nil {
		t.Fatalf("expected conflicting flag error")
	}
}

func TestUpdateServiceSurfacesInstallFailure(t *testing.T) {
	updater := &fakeUpdater{installErr: errors.New("missing brew")}
	svc := NewUpdateService(updater)

	_, err := svc.Run(domain.UpdateInput{Check: true})
	if err == nil {
		t.Fatalf("expected install error")
	}
}
