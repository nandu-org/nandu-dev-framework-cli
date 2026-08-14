package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestCustomizableAction pins the marker-independent decision table for
// user_customizable files (the #11 safeguard).
func TestCustomizableAction(t *testing.T) {
	cases := []struct{ current, manifest, want string }{
		{"", "abc", "create"},           // absent on disk -> deliver placeholder
		{"abc", "abc", "skip"},          // disk == framework -> nothing to do
		{"deadbeef", "abc", "preserve"}, // disk differs (client-owned) -> never overwrite
	}
	for _, c := range cases {
		if got := customizableAction(c.current, c.manifest); got != c.want {
			t.Errorf("customizableAction(%q,%q)=%q want %q", c.current, c.manifest, got, c.want)
		}
	}
}

// TestHandleUserCustomizablePreservesCustomization is the synthetic reproduction
// of the field-note bug (ndf update overwriting a customized hook script). It
// asserts the safeguard preserves a client-customized file and is a no-op when
// the file already matches the framework — exercising the no-network paths.
// Crucially, no marker is consulted at all, so the protection holds even when
// the marker's installed_checksums entry is missing or stale.
func TestHandleUserCustomizablePreservesCustomization(t *testing.T) {
	dir := t.TempDir()
	// f.Path is project-relative; cmdUpdate runs from the project root. Chdir
	// (manual + defer, not t.Chdir, to stay compatible with the module's
	// declared go1.22).
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	rel := ".claude/hooks/pre-commit-tests.sh"
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := []byte("#!/usr/bin/env bash\nmake test\n")
	if err := os.WriteFile(rel, custom, 0o755); err != nil {
		t.Fatal(err)
	}

	// (1) Manifest checksum differs from the client's file -> preserve, untouched.
	f := ManifestFile{Path: rel, Checksum: "placeholder_sha_that_differs", UserCustomizable: true}
	var changes []change
	var preserved []string
	handleUserCustomizable("unused-ref", f, &changes, &preserved)

	if got, _ := os.ReadFile(rel); string(got) != string(custom) {
		t.Fatalf("customized file was modified: got %q", got)
	}
	if len(preserved) != 1 || preserved[0] != rel {
		t.Fatalf("expected %s preserved, got %v", rel, preserved)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes recorded, got %v", changes)
	}

	// (2) File already equals the framework version -> skip (no-op).
	f2 := ManifestFile{Path: rel, Checksum: sha256OfFileOrEmpty(rel), UserCustomizable: true}
	var changes2 []change
	var preserved2 []string
	handleUserCustomizable("unused-ref", f2, &changes2, &preserved2)
	if len(preserved2) != 0 || len(changes2) != 0 {
		t.Fatalf("matching file should be a no-op; preserved=%v changes=%v", preserved2, changes2)
	}
}

// TestLedgerAfterUpdate pins the delivered-keyed ledger rule (marker.
// writeMarker docstring, half 2): the install record asserts only what was
// actually delivered. The four cases mirror the docstring's list. The
// regression this guards: recording the whole target manifest regardless of
// outcome made a skipped file permanently invisible — every later update
// short-circuited on manifest == installed and the project ran an old file
// under a current version number.
func TestLedgerAfterUpdate(t *testing.T) {
	files := []ManifestFile{
		{Path: "a.md", Checksum: "newA"},
		{Path: "b.md", Checksum: "newB"},
		{Path: "c.md", Checksum: "newC"}, // net-new this release
		{Path: "renamed-new.md", Checksum: "newR", RenamedFrom: "renamed-old.md"},
	}
	prev := map[string]string{
		"a.md":           "oldA",
		"b.md":           "oldB",
		"renamed-old.md": "oldR",
		"gone.md":        "oldG", // removed from the manifest this release
	}

	t.Run("all delivered", func(t *testing.T) {
		got := ledgerAfterUpdate(files, prev, nil)
		want := map[string]string{"a.md": "newA", "b.md": "newB", "c.md": "newC", "renamed-new.md": "newR"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v; want %v", got, want)
		}
		if _, still := got["gone.md"]; still {
			t.Error("removed file must drop out of the ledger")
		}
	})

	t.Run("conflict skip carries the previous installed value forward", func(t *testing.T) {
		got := ledgerAfterUpdate(files, prev, []string{"b.md"})
		if got["b.md"] != "oldB" {
			t.Errorf("skipped b.md = %q; want the carried-forward %q — recording %q would hide the skip forever", got["b.md"], "oldB", "newB")
		}
		if got["a.md"] != "newA" {
			t.Errorf("delivered a.md = %q; want %q", got["a.md"], "newA")
		}
	})

	t.Run("net-new collision skip gets no entry", func(t *testing.T) {
		got := ledgerAfterUpdate(files, prev, []string{"c.md"})
		if _, has := got["c.md"]; has {
			t.Errorf("skipped net-new c.md got entry %q; want none (stays new, collision prompt re-fires)", got["c.md"])
		}
	})

	t.Run("rename skip keeps the source entry and gives the target none", func(t *testing.T) {
		got := ledgerAfterUpdate(files, prev, []string{"renamed-old.md", "renamed-new.md"})
		if got["renamed-old.md"] != "oldR" {
			t.Errorf("kept rename source = %q; want carried-forward %q so the rename re-offers", got["renamed-old.md"], "oldR")
		}
		if _, has := got["renamed-new.md"]; has {
			t.Error("skipped rename target must get no entry")
		}
	})
}

// TestLedgerAfterUpdatePreservedCustomizable pins the preserved-file rule: a
// user_customizable file the update chose not to touch joins the
// not-delivered carry list, so its entry keeps the last delivered
// placeholder reference instead of silently advancing to the new manifest's
// value (recording intent). Guards the re-arm scenario: if the file later
// loses its flag with unchanged shipped content, a silently-advanced entry
// would short-circuit the update and hide the client's divergence again.
func TestLedgerAfterUpdatePreservedCustomizable(t *testing.T) {
	files := []ManifestFile{{Path: "hook.sh", Checksum: "newPlaceholder", UserCustomizable: true}}
	prev := map[string]string{"hook.sh": "oldPlaceholder"}
	got := ledgerAfterUpdate(files, prev, []string{"hook.sh"})
	if got["hook.sh"] != "oldPlaceholder" {
		t.Errorf("preserved customizable file = %q; want carried-forward %q", got["hook.sh"], "oldPlaceholder")
	}
}

// TestShouldRemoveRetiredPath pins pass 2's guard: only content exactly
// matching the last delivery may be deleted; anything else is the
// client's (edited, or kept at a declined-rename prompt) and is left.
func TestShouldRemoveRetiredPath(t *testing.T) {
	if !shouldRemoveRetiredPath("sha-a", "sha-a") {
		t.Error("unmodified retired file must be removable")
	}
	if shouldRemoveRetiredPath("sha-a", "sha-b") {
		t.Error("modified retired file must never be removed")
	}
	if shouldRemoveRetiredPath("sha-a", "") {
		t.Error("an already-absent retired file has nothing to remove")
	}
}

// TestDedupSorted pins the marker-list normalizer: unique, sorted, nil on
// empty (stable, idempotent marker rewrites).
func TestDedupSorted(t *testing.T) {
	got := dedupSorted([]string{"b", "a", "b", "c", "a"})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("dedupSorted = %v; want [a b c]", got)
	}
	if dedupSorted(nil) != nil {
		t.Error("dedupSorted(nil) must be nil")
	}
}

// TestCustomizablePathsNeverNil pins the nil-vs-empty contract's producer
// side: a manifest with zero flagged files must yield an EMPTY list, not
// nil — nil marshals to null, which reads back as "pre-v2.9.0 record" and
// would trap such an install behind a refresh note no update can satisfy.
func TestCustomizablePathsNeverNil(t *testing.T) {
	m := &Manifest{Files: []ManifestFile{{Path: "a.md", Checksum: "x"}}}
	got := m.customizablePaths()
	if got == nil {
		t.Fatal("customizablePaths on a flag-less manifest must be non-nil empty")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

// TestUpdateFileAction pins pass 1's routing table, including the branch
// ORDER: restore beats everything; disk-already-matching beats the
// silent-replace re-fetch (the pristine-checkout self-heal must not
// produce a network fetch and a phantom change entry); a pending mark
// overrules the recorded-current short-circuit; a client's edit of a
// recorded-current file stays untouched (v1.2.2 doctrine).
func TestUpdateFileAction(t *testing.T) {
	const inst, cur, man = "sha-installed", "sha-current", "sha-manifest"
	cases := []struct {
		name                     string
		installed, current, mani string
		pending                  bool
		want                     fileAction
	}{
		{"vanished file restores", inst, "", man, false, actionRestore},
		{"vanished + pending still restores", man, "", man, true, actionRestore},
		{"recorded current, untouched", man, man, man, false, actionNoop},
		{"recorded current, client edited — untouched until next delta", man, cur, man, false, actionNoop},
		{"pending + pristine disk self-heals with no fetch", man, man, man, true, actionRecordDelivered},
		{"pending + recorded current + edited disk — conflict", man, cur, man, true, actionConflict},
		{"disk equals incoming (hand-updated) — record, no fetch", inst, man, man, false, actionRecordDelivered},
		{"client unchanged since delivery — silent replace", inst, inst, man, false, actionSilentReplace},
		{"three-way divergence — conflict", inst, cur, man, false, actionConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := updateFileAction(c.installed, c.current, c.mani, c.pending); got != c.want {
				t.Errorf("updateFileAction(%q,%q,%q,%v) = %q; want %q", c.installed, c.current, c.mani, c.pending, got, c.want)
			}
		})
	}
}

// TestKeepPendingMark pins the mark-survival policy: a typed skip on a
// recorded path is a decision (clears); an unattended auto-skip is not
// (keeps); an entry-less decline keeps regardless — its mark is its only
// visibility.
func TestKeepPendingMark(t *testing.T) {
	if keepPendingMark(true, true) {
		t.Error("typed skip on a recorded path must clear the mark")
	}
	if !keepPendingMark(true, false) {
		t.Error("auto-skip must keep the mark (a default is not a decision)")
	}
	if !keepPendingMark(false, true) || !keepPendingMark(false, false) {
		t.Error("entry-less declines must always keep the mark")
	}
}

// TestConflictSkipOutcome pins the three per-case truths a conflict skip
// prints — the version-delta re-surface promise, the typed re-offer
// decline's finality, and the not-decided (auto-skip or failed action)
// stays-marked message — plus the summary suffix each contributes, so the
// end-of-run list carries the outcome per line instead of a blanket claim.
func TestConflictSkipOutcome(t *testing.T) {
	msg, suffix := conflictSkipOutcome(true, true)
	if !strings.Contains(msg, "not be offered again") || !strings.Contains(suffix, "--fix") {
		t.Errorf("typed re-offer decline must state finality + the --fix re-open route; got %q / %q", msg, suffix)
	}
	msg, suffix = conflictSkipOutcome(true, false)
	if !strings.Contains(msg, "stays marked") || !strings.Contains(suffix, "stays marked") {
		t.Errorf("undecided re-offer skip must say the mark survives; got %q / %q", msg, suffix)
	}
	msg, suffix = conflictSkipOutcome(false, true)
	if !strings.Contains(msg, "re-surface") || !strings.Contains(suffix, "re-offered on the next update") {
		t.Errorf("version-delta skip must promise the re-surface; got %q / %q", msg, suffix)
	}
}
