package git

import "testing"

func TestParseWorktreesParsesMultipleEntries(t *testing.T) {
	input := "" +
		"worktree /tmp/repo\n" +
		"HEAD 1234567\n" +
		"branch refs/heads/main\n" +
		"\n" +
		"worktree /tmp/repo-wt\n" +
		"HEAD 89abcde\n" +
		"branch refs/heads/feature/demo\n" +
		"locked maintenance\n" +
		"\n"

	entries := parseWorktrees(input)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Path != "/tmp/repo" || entries[0].Branch != "main" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Path != "/tmp/repo-wt" || entries[1].Branch != "feature/demo" {
		t.Fatalf("unexpected second entry: %+v", entries[1])
	}
	if !entries[1].IsLocked || entries[1].PruneReason != "maintenance" {
		t.Fatalf("expected locked maintenance, got %+v", entries[1])
	}
}
