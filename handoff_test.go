package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestStagePathsForCommit pins the scoped-staging rule: the post-update
// commit stages exactly this run's own changes plus the CLI-state subtree —
// never the whole tree. The regression this guards: `git add -A` swept
// whatever else sat in the working tree into a commit labeled as a framework
// update (and, in the incident that motivated v2.9.0, pushed it).
func TestStagePathsForCommit(t *testing.T) {
	changes := []change{
		{Kind: changeUpdate, Path: "CLAUDE.md"},
		{Kind: changeNew, Path: ".claude/ndf/ask-formats.md"},
		{Kind: changeUpdateWithConflict, Path: ".claude/commands/planning.md"},
		{Kind: changeRename, Path: "old-name.md → new-name.md"},
		{Kind: changeRemove, Path: "dead-file.md"},
		{Kind: changeUpdate, Path: "CLAUDE.md"}, // duplicate must not repeat
	}
	got := stagePathsForCommit(changes)
	want := []string{
		"CLAUDE.md",
		".claude/ndf/ask-formats.md",
		".claude/commands/planning.md",
		"old-name.md",
		"new-name.md",
		"dead-file.md",
		".ndf/cli",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stagePathsForCommit = %v; want %v", got, want)
	}
}

// TestStagePathsForCommitEmpty: even with no file changes (e.g. a
// marker-only rewrite), the CLI-state subtree is staged — the marker itself
// is part of what coworkers must receive.
func TestStagePathsForCommitEmpty(t *testing.T) {
	got := stagePathsForCommit(nil)
	if !reflect.DeepEqual(got, []string{".ndf/cli"}) {
		t.Errorf("stagePathsForCommit(nil) = %v; want [.ndf/cli]", got)
	}
}

// TestCommitPushConsented pins the consent rule the incident turns on: only
// an explicit yes runs git. The empty string IS the prompt default — before
// v2.9.0 it meant yes, and an unattended update (prompts auto-answer their
// default without a TTY) committed and pushed with nobody deciding.
func TestCommitPushConsented(t *testing.T) {
	for _, c := range []struct {
		choice string
		want   bool
	}{
		{"", false}, // the default — MUST decline
		{"n", false},
		{"no", false},
		{"skip", false},
		{"anything-else", false},
		{"Y", true},
		{"y", true},
		{"yes", true},
		{" YES ", true},
	} {
		if got := commitPushConsented(c.choice); got != c.want {
			t.Errorf("commitPushConsented(%q) = %v; want %v", c.choice, got, c.want)
		}
	}
}

// TestOnIntegrationBranch pins the branch classification driving the
// non-main caution (which must print on BOTH the interactive and the
// unattended path).
func TestOnIntegrationBranch(t *testing.T) {
	for b, want := range map[string]bool{"main": true, "master": true, "develop": false, "dev": false, "": false} {
		if got := onIntegrationBranch(b); got != want {
			t.Errorf("onIntegrationBranch(%q) = %v; want %v", b, got, want)
		}
	}
}

// TestPrintTeamHandoff pins the block's honesty rules: the real branch in
// every line (no hardcoded main), a NOT-PUSHED banner only when
// committable changes actually exist, the undelivered section inside the
// fence, and full suppression (with an explanation) when there is no
// branch to name.
func TestPrintTeamHandoff(t *testing.T) {
	changes := []change{{Kind: changeUpdate, Path: "a.md"}}

	var b strings.Builder
	printTeamHandoff(&b, "4.19.0", "4.20.0", changes, 0, "develop", false, true, []string{"b.md (skipped)"})
	out := b.String()
	for _, want := range []string{
		"NOT YET PUSHED",
		"pushed to origin/develop",
		"(on develop)",
		"git pull origin develop",
		"Not delivered on this run (1 file(s)",
		"b.md (skipped)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("handoff missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "origin/main") || strings.Contains(out, "pull origin main") {
		t.Errorf("handoff hardcodes main:\n%s", out)
	}

	b.Reset()
	printTeamHandoff(&b, "4.20.0", "4.20.0", changes, 0, "main", true, false, nil)
	out = b.String()
	if strings.Contains(out, "NOT YET PUSHED") {
		t.Errorf("pushed run must not carry the banner:\n%s", out)
	}
	if !strings.Contains(out, "Framework updated on main (drift fixes") {
		t.Errorf("same-version headline missing:\n%s", out)
	}

	// Restore-only run: changes exist but the tree ended clean — no banner.
	b.Reset()
	printTeamHandoff(&b, "4.20.0", "4.20.0", changes, 0, "main", false, false, nil)
	if strings.Contains(b.String(), "NOT YET PUSHED") {
		t.Errorf("clean-tree run must not carry the banner:\n%s", b.String())
	}

	// No branch: the shareable block is suppressed with an explanation.
	b.Reset()
	printTeamHandoff(&b, "4.19.0", "4.20.0", changes, 0, "", false, true, nil)
	out = b.String()
	if strings.Contains(out, "TEAM HANDOFF") || strings.Contains(out, "main") {
		t.Errorf("branchless run must not print a shareable block or invent main:\n%s", out)
	}
	if !strings.Contains(out, "No team handoff printed") {
		t.Errorf("branchless run must explain the suppression:\n%s", out)
	}
}
