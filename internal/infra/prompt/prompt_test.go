package prompt

import (
	"bufio"
	"strings"
	"testing"
)

func TestConfirmWithTokenRequiresExactToken(t *testing.T) {
	a := &Adapter{in: bufio.NewReader(strings.NewReader("APPLY\n"))}
	ok, err := a.ConfirmWithToken("Dry plan ready.", "APPLY", false, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatalf("expected confirmation true")
	}
}

func TestConfirmWithTokenFailsInNonInteractiveWithoutYes(t *testing.T) {
	a := &Adapter{in: bufio.NewReader(strings.NewReader(""))}
	_, err := a.ConfirmWithToken("Dry plan ready.", "APPLY", true, false)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestSelectOneChoosesIndexedOption(t *testing.T) {
	a := &Adapter{in: bufio.NewReader(strings.NewReader("2\n"))}
	got, err := a.SelectOne("Select root", []string{"a", "b"}, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "b" {
		t.Fatalf("expected b, got %q", got)
	}
}
