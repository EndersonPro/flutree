package ui

import "testing"

func TestStartLoadingReturnsStopFunctionInNonTTY(t *testing.T) {
	stop := StartLoading("Running pub get across workspace...")
	if stop == nil {
		t.Fatalf("expected non-nil stop callback")
	}
	stop(true)
	stop(false)
}
