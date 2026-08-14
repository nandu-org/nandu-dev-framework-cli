package main

import (
	"testing"
)

// fakeDisk builds a diskSha lookup from a fixed map ("" = absent).
func fakeDisk(m map[string]string) func(string) string {
	return func(p string) string { return m[p] }
}

// TestClassifyInstalled pins the verify verdict table, including the case
// the command exists for: a file whose recorded hash claims one version
// while the disk carries another (the "ledger asserts an installation that
// never happened" incident shape) must read DRIFTED — and a customized
// client-owned file must NOT, or every healthy project alarms forever.
func TestClassifyInstalled(t *testing.T) {
	installed := map[string]string{
		"ok.md":         "aaa",
		"drifted.md":    "bbb",
		"customized.sh": "ccc",
		"missing.md":    "ddd",
	}
	disk := fakeDisk(map[string]string{
		"ok.md":         "aaa",
		"drifted.md":    "OLD-VERSION-HASH", // e.g. a file an update never delivered
		"customized.sh": "CLIENT-EDIT-HASH",
		// missing.md absent
	})
	customizable := map[string]struct{}{"customized.sh": {}}

	got := classifyInstalled(installed, disk, customizable, nil)
	want := map[string]verifyStatus{
		"ok.md":         verifyOK,
		"drifted.md":    verifyDrifted,
		"customized.sh": verifyCustomized,
		"missing.md":    verifyMissing,
	}
	for p, w := range want {
		if got[p] != w {
			t.Errorf("%s: got %q; want %q", p, got[p], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("classified %d paths; want %d", len(got), len(want))
	}
}

// TestClassifyInstalledCustomizedButUntouched: a user_customizable file the
// client never actually customized (disk still matches the delivered
// placeholder) reads ok, not customized — the flag only excuses divergence,
// it does not invent one.
func TestClassifyInstalledCustomizedButUntouched(t *testing.T) {
	got := classifyInstalled(
		map[string]string{"hook.sh": "aaa"},
		fakeDisk(map[string]string{"hook.sh": "aaa"}),
		map[string]struct{}{"hook.sh": {}},
		nil,
	)
	if got["hook.sh"] != verifyOK {
		t.Errorf("untouched customizable file: got %q; want %q", got["hook.sh"], verifyOK)
	}
}

// TestClassifyInstalledPendingAndUnclassified pins the two v2.9.0 verdicts
// added after review: a path in the pending re-offer set reads
// awaiting-decision (even with NO ledger entry — the declined-net-new
// case, whose mark is its only visibility), and a NIL customizable set
// (record predates the field, or an older CLI stripped it) degrades every
// divergence — including a missing file, which may be a deleted
// client-owned hook — to unclassified rather than guessing DRIFTED.
// Precedence pins: customizable beats pending (a client-owned file's
// re-offer is moot — the permanently-stuck-AWAITING regression), and an
// empty recorded checksum must never read ok for an absent file.
func TestClassifyInstalledPendingAndUnclassified(t *testing.T) {
	installed := map[string]string{
		"pending.md":   "old-delivered",
		"differs.md":   "recorded",
		"absent.md":    "recorded2",
		"matches.md":   "same",
		"empty-rec.md": "",
	}
	disk := fakeDisk(map[string]string{
		"pending.md": "whatever",
		"differs.md": "something-else",
		"matches.md": "same",
	})
	pending := map[string]struct{}{"pending.md": {}, "entryless.md": {}}

	// nil customizable set: unclassified for every unexplained divergence;
	// pending still classifies (the mark is v2.9.0-authored by definition).
	got := classifyInstalled(installed, disk, nil, pending)
	want := map[string]verifyStatus{
		"pending.md":   verifyAwaiting,
		"entryless.md": verifyAwaiting, // no ledger entry — union walk
		"differs.md":   verifyUnclassified,
		"absent.md":    verifyUnclassified,
		"matches.md":   verifyOK,
		"empty-rec.md": verifyUnclassified, // "" recorded must not read ok
	}
	for p, w := range want {
		if got[p] != w {
			t.Errorf("nil set, %s: got %q; want %q", p, got[p], w)
		}
	}

	// non-nil set: definitive verdicts return; empty recorded stays not-ok.
	got = classifyInstalled(installed, disk, map[string]struct{}{}, pending)
	if got["differs.md"] != verifyDrifted || got["absent.md"] != verifyMissing {
		t.Errorf("empty non-nil set: differs=%q absent=%q; want drifted/missing", got["differs.md"], got["absent.md"])
	}
	if got["empty-rec.md"] != verifyMissing {
		t.Errorf("empty recorded checksum on absent file: got %q; want %q", got["empty-rec.md"], verifyMissing)
	}

	// customizable beats pending: the re-offer question is moot for a
	// client-owned file (this is the permanently-stuck-AWAITING guard).
	got = classifyInstalled(installed, disk, map[string]struct{}{"pending.md": {}}, pending)
	if got["pending.md"] != verifyCustomized {
		t.Errorf("customizable+pending: got %q; want %q", got["pending.md"], verifyCustomized)
	}
}

// TestClassifyInstalledCustomizedMissing: a deleted user_customizable file
// is the client's act on a file the framework disclaims — customized, never
// missing-as-drift (it would otherwise nag in `ndf version` forever).
func TestClassifyInstalledCustomizedMissing(t *testing.T) {
	got := classifyInstalled(
		map[string]string{"hook.sh": "aaa"},
		fakeDisk(map[string]string{}),
		map[string]struct{}{"hook.sh": {}},
		nil,
	)
	if got["hook.sh"] != verifyCustomized {
		t.Errorf("deleted customizable file: got %q; want %q", got["hook.sh"], verifyCustomized)
	}
}

// TestVerifyExitCode pins the documented contract: 0 = clean or
// unclassified-only, 1 = drifted/missing/awaiting, and any --fix run that
// completed (marked files, or found them already marked) exits 0 so the
// documented `ndf verify --fix && ndf update` chain stays idempotent.
func TestVerifyExitCode(t *testing.T) {
	if got := verifyExitCode(map[verifyStatus]int{verifyOK: 5}, false); got != 0 {
		t.Errorf("clean: got %d; want 0", got)
	}
	if got := verifyExitCode(map[verifyStatus]int{verifyUnclassified: 3}, false); got != 0 {
		t.Errorf("unclassified-only: got %d; want 0", got)
	}
	for _, s := range []verifyStatus{verifyDrifted, verifyMissing, verifyAwaiting} {
		if got := verifyExitCode(map[verifyStatus]int{s: 1}, false); got != 1 {
			t.Errorf("%s: got %d; want 1", s, got)
		}
	}
	if got := verifyExitCode(map[verifyStatus]int{verifyAwaiting: 2}, true); got != 0 {
		t.Errorf("--fix completed with awaiting files: got %d; want 0 (the chain must reach ndf update)", got)
	}
}

// TestFrameworkDriftCountNilVsEmpty pins the version-hint gate: a nil
// customizable list (pre-v2.9.0 record) returns -1 — cannot classify, hint
// replaced by the cannot-check line — while an empty non-nil list counts
// normally. Pending marks count as drift so the hint survives a pulled
// re-offer until an update resolves it.
func TestFrameworkDriftCountNilVsEmpty(t *testing.T) {
	disk := fakeDisk(map[string]string{"a.md": "actual"})
	nilRecord := &Marker{InstalledChecksums: map[string]string{"a.md": "recorded"}}
	if got := frameworkDriftCount(nilRecord, disk); got != -1 {
		t.Errorf("nil list: got %d; want -1", got)
	}
	emptyRecord := &Marker{InstalledChecksums: map[string]string{"a.md": "recorded"}, UserCustomizable: []string{}}
	if got := frameworkDriftCount(emptyRecord, disk); got != 1 {
		t.Errorf("empty list, drifted file: got %d; want 1", got)
	}
	pendingRecord := &Marker{InstalledChecksums: map[string]string{}, UserCustomizable: []string{}, PendingReoffer: []string{"b.md"}}
	if got := frameworkDriftCount(pendingRecord, disk); got != 1 {
		t.Errorf("pending-only: got %d; want 1", got)
	}
}
