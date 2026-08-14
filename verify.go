package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// `ndf verify` — offline integrity check of the installed framework files
// against the marker's install record.
//
// Why it exists: the marker is a claim, and for a long time nothing ever
// checked it. A 2026-07 field diagnosis found a client project whose marker
// (and therefore `ndf version`) reported a current framework while four
// framework files on disk were seven releases old — an earlier update had
// not delivered them, and every later update short-circuited on the marker
// entry. This command makes that state visible in seconds, with no network.
//
// Classification per installed_checksums entry, disk hash vs recorded hash:
//
//	ok                — disk matches the last delivered version
//	customized        — differs (or absent), but the path is flagged
//	                    user_customizable: client-owned by design, not drift
//	unclassified      — differs, but the record predates the
//	                    user_customizable list, so ownership is unknowable
//	                    offline; refresh via one `ndf update`
//	awaiting decision — the path carries a pending re-offer mark (see
//	                    verifyAwaiting for the writer list); the next
//	                    interactive `ndf update` prompts for it
//	DRIFTED           — differs and is framework-owned: either the client
//	                    edited it, or a past update failed to deliver it
//	MISSING           — no file on disk (the next update restores it)
//
// Exit codes (verifyExitCode, unit-tested): 0 = nothing
// drifted/missing/awaiting (unclassified counts as 0 — a legacy record must
// not fail red on every real project's customized files; an already-marked
// --fix rerun also exits 0 so the documented `ndf verify --fix && ndf
// update` chain stays idempotent); 1 = drift verdict; 2 = could not run
// (no project marker, corrupt marker, unknown flag, --fix without a
// terminal — mirroring `ndf is-project`'s convention so a pipeline can
// tell "drift found" from "check never ran").
//
// `--fix` (interactive only) adds each drifted path to the marker's
// pending_reoffer list so the next `ndf update` re-offers it through a
// diff prompt. Safe for both drift causes — a stale delivery shows the
// framework's changes and the user picks replace; a real customization
// shows their edits and the user picks skip or backup. Three rejected
// alternatives, recorded on purpose: writing the DISK hash into the ledger
// is the v1.2.2 corruption shape (the next update would silently replace a
// genuine customization); DELETING the entry makes the file invisible to
// verify/version — a false all-clear — and exposes it to a blind re-fetch
// if a later manifest declares the path as a rename source; an IN-BAND
// sentinel value destroyed the last-delivered checksum (which pass 2's
// removal guard and the rename fast-path compare against) and, in the
// committed team-shared marker, produced phantom prompts on teammates'
// pristine checkouts (see Marker.PendingReoffer).

// verifyStatus is one file's verdict.
type verifyStatus string

const (
	verifyOK         verifyStatus = "ok"
	verifyCustomized verifyStatus = "customized"
	verifyDrifted    verifyStatus = "drifted"
	verifyMissing    verifyStatus = "missing"
	// verifyAwaiting: the path carries a pending re-offer mark — written by
	// `verify --fix`; created or kept by any skip or failed action that was
	// not a typed human decision; and by any declined net-new collision,
	// typed or not (entry-less paths keep the mark as their only
	// visibility — see keepPendingMark). The next interactive `ndf update`
	// prompts for it.
	verifyAwaiting verifyStatus = "awaiting decision"
	// verifyUnclassified: the file differs but the record predates the
	// user_customizable list (or an older CLI stripped it), so "your file by
	// design" vs "real drift" cannot be told apart offline. Not counted as
	// drift — on a pre-v2.9.0 record essentially every real project has
	// customized files, and a first `ndf verify` that fails red on all of
	// them would train operators to ignore the command.
	verifyUnclassified verifyStatus = "unclassified"
)

// classifyInstalled computes the per-path verify verdicts from the marker's
// install record, a disk-hash lookup (path → sha256, "" when absent), the
// customizable path set (nil = record predates the list; unknown), and the
// pending re-offer set. It walks the UNION of recorded and pending paths —
// a declined net-new collision has a pending mark but no ledger entry, and
// must not be invisible. Precedence: customizable beats awaiting (a
// client-owned file's re-offer question is moot); awaiting beats the
// drift/ok verdicts (an update prompt is already scheduled); the ok match
// requires a non-empty recorded value (an empty recorded checksum must not
// read as ok for an absent file). A client deleting a customizable file is
// theirs to do, not drift. Pure — unit-tested in verify_test.go.
func classifyInstalled(installed map[string]string, diskSha func(string) string, customizable map[string]struct{}, pending map[string]struct{}) map[string]verifyStatus {
	out := make(map[string]verifyStatus, len(installed)+len(pending))
	classify := func(path string) verifyStatus {
		recorded, hasEntry := installed[path]
		actual := diskSha(path)
		_, isCustomizable := customizable[path]
		_, isPending := pending[path]
		switch {
		case isCustomizable:
			if hasEntry && actual == recorded && recorded != "" {
				return verifyOK
			}
			return verifyCustomized
		case isPending:
			return verifyAwaiting
		case hasEntry && actual == recorded && recorded != "":
			return verifyOK
		case customizable == nil:
			// Differs or missing, but the record cannot say whose file this
			// is — even a missing file may be a deleted client-owned hook.
			return verifyUnclassified
		case actual == "":
			return verifyMissing
		default:
			return verifyDrifted
		}
	}
	for path := range installed {
		out[path] = classify(path)
	}
	for path := range pending {
		if _, done := out[path]; !done {
			out[path] = classify(path)
		}
	}
	return out
}

// projectFileSha hashes a project-relative path through the shared
// override-aware resolver, so verify/version classify the same project the
// marker was read from even when $CLAUDE_PROJECT_DIR points elsewhere.
func projectFileSha(rel string) string {
	return sha256OfFileOrEmpty(resolveProjectPath(rel))
}

// frameworkDriftCount returns how many framework-owned recorded files
// differ from the install record on disk (drifted, missing, or awaiting a
// re-offer) — the number `ndf version` surfaces as its hint line. Returns
// -1 when the record cannot be classified: a NIL user_customizable list
// means the marker predates v2.9.0 (or an older CLI's writeMarker stripped
// the field on its last update), and without it a client's own customized
// files would be miscounted as drift. An empty non-nil list is a real
// classification: a manifest that flags nothing.
func frameworkDriftCount(m *Marker, diskSha func(string) string) int {
	if m.UserCustomizable == nil {
		return -1
	}
	customizable := make(map[string]struct{}, len(m.UserCustomizable))
	for _, p := range m.UserCustomizable {
		customizable[p] = struct{}{}
	}
	pending := make(map[string]struct{}, len(m.PendingReoffer))
	for _, p := range m.PendingReoffer {
		pending[p] = struct{}{}
	}
	n := 0
	for _, s := range classifyInstalled(m.InstalledChecksums, diskSha, customizable, pending) {
		if s == verifyDrifted || s == verifyMissing || s == verifyAwaiting {
			n++
		}
	}
	return n
}

// verifyExitCode maps the classification counts to the command's exit
// code. fixRan reports whether --fix executed (marking files or finding
// them already marked): a successful or already-satisfied --fix exits 0 so
// `ndf verify --fix && ndf update` chains idempotently; without --fix, any
// drifted/missing/awaiting verdict exits 1. Unclassified never fails the
// command. Pure — unit-tested.
func verifyExitCode(counts map[verifyStatus]int, fixRan bool) int {
	if fixRan {
		return 0
	}
	if counts[verifyDrifted] > 0 || counts[verifyMissing] > 0 || counts[verifyAwaiting] > 0 {
		return 1
	}
	return 0
}

// cmdVerify is the entry point for `ndf verify`.
func cmdVerify(args []string) {
	fix := false
	for _, a := range args {
		switch a {
		case "--fix":
			fix = true
		case "-h", "--help":
			printHelpVerify()
			return
		default:
			warn("unknown verify flag: %s", a)
			os.Exit(2)
		}
	}

	// Read command: honors $CLAUDE_PROJECT_DIR like is-project/version (so
	// hooks and slash commands can invoke it from any cwd). --fix writes
	// only the marker, through the same override-aware resolver, so it is
	// self-consistent without anchoring — the config-set precedent.
	//
	// Deliberately NOT requireMarker: its die() exits 1, and this command's
	// exit 1 means "drift found". "Could not run" — no project, corrupt
	// marker — must be distinguishable for pipelines, so it exits 2 here
	// (the `ndf is-project` convention).
	marker, err := loadMarker()
	if err != nil {
		warn("%v", err)
		os.Exit(2)
	}
	if marker == nil {
		warn("no %s found in current directory. This isn't an ndf project — run `ndf init` first.", projectMarker)
		os.Exit(2)
	}

	// nil set = record predates the classification (classifyInstalled
	// degrades to "unclassified" rather than guessing); empty non-nil set =
	// a manifest that flags nothing.
	var customizable map[string]struct{}
	if marker.UserCustomizable != nil {
		customizable = make(map[string]struct{}, len(marker.UserCustomizable))
		for _, p := range marker.UserCustomizable {
			customizable[p] = struct{}{}
		}
	}
	pending := make(map[string]struct{}, len(marker.PendingReoffer))
	for _, p := range marker.PendingReoffer {
		pending[p] = struct{}{}
	}

	statuses := classifyInstalled(marker.InstalledChecksums, projectFileSha, customizable, pending)

	if len(statuses) == 0 {
		warn("the install record lists no files — nothing to verify. Run `ndf update` to (re)establish it.")
		os.Exit(2)
	}

	// Stable output order.
	paths := make([]string, 0, len(statuses))
	for p := range statuses {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	counts := map[verifyStatus]int{}
	var drifted []string
	info("verifying %d file(s) against framework v%s's install record…", len(paths), marker.Version)
	for _, p := range paths {
		st := statuses[p]
		counts[st]++
		switch st {
		case verifyOK:
			// Silent — listing every healthy file buries the signal.
		case verifyCustomized:
			if projectFileSha(p) == "" {
				info("  customized (yours; absent — the next `ndf update` re-creates the placeholder): %s", p)
			} else {
				info("  customized (yours): %s", p)
			}
		case verifyUnclassified:
			info("  unclassified: %s", p)
		case verifyAwaiting:
			warn("  AWAITING DECISION: %s — run `ndf update` and decide it at the prompt ([r] takes the framework version, [s]kip keeps yours; either way the question is answered)", p)
		case verifyDrifted:
			warn("  DRIFTED: %s", p)
			drifted = append(drifted, p)
		case verifyMissing:
			warn("  MISSING: %s", p)
		}
	}

	summary := []string{}
	for _, st := range []verifyStatus{verifyOK, verifyCustomized, verifyUnclassified, verifyAwaiting, verifyDrifted, verifyMissing} {
		if counts[st] > 0 {
			summary = append(summary, sprintfCount(counts[st], string(st)))
		}
	}
	info("%s", strings.Join(summary, ", "))

	// Per-verdict guidance — each line only when its verdict occurred.
	if counts[verifyDrifted] > 0 {
		warn("a drifted file means either you edited it, or a past update never delivered it.")
	}
	if counts[verifyMissing] > 0 {
		warn("a missing file is restored by the next `ndf update`.")
	}
	if counts[verifyAwaiting] > 0 {
		warn("awaiting-decision file(s) are prompted at the next `ndf update` (in an interactive terminal).")
	}

	if fix {
		runVerifyFix(marker, drifted, counts)
		os.Exit(verifyExitCode(counts, true))
	}

	if counts[verifyDrifted] > 0 {
		warn("to resolve drift: run `ndf verify --fix` in an interactive terminal — it re-offers each")
		warn("drifted file on the next `ndf update`, with a diff, so you decide file by file.")
	}

	// The legacy-record note goes LAST: on a pre-v2.9.0 record it is the
	// operative fact, and a trailing green-looking line would bury it.
	if marker.UserCustomizable == nil && counts[verifyUnclassified] > 0 {
		warn("%d file(s) could not be classified — this install record predates CLI v2.9.0 and cannot", counts[verifyUnclassified])
		warn("confirm they are current. Run `ndf update` once to refresh the record, then re-run `ndf verify`.")
	} else if counts[verifyDrifted] == 0 && counts[verifyMissing] == 0 && counts[verifyAwaiting] == 0 {
		if counts[verifyCustomized] > 0 {
			ok("no drift: every framework-owned file matches what was delivered (%d file(s) are yours by design).", counts[verifyCustomized])
		} else {
			ok("install record verified: files on disk match what the framework last delivered.")
		}
	}

	os.Exit(verifyExitCode(counts, false))
}

// runVerifyFix implements --fix: interactive-only, idempotent, and it
// never touches installed_checksums — re-offer intent lives in the
// marker's pending_reoffer list (see the head comment for the two
// rejected in-record designs).
func runVerifyFix(marker *Marker, drifted []string, counts map[verifyStatus]int) {
	if !isStdinTTY() {
		warn("verify --fix needs an interactive terminal (the fix routes each file through a diff prompt on the next update).")
		os.Exit(2)
	}
	if marker.UserCustomizable == nil {
		warn("--fix has nothing to act on until the record is refreshed — run `ndf update` once, then re-run `ndf verify --fix`.")
		os.Exit(2)
	}
	if len(drifted) == 0 {
		if counts[verifyAwaiting] > 0 {
			ok("%d file(s) are already marked for re-offer — run `ndf update` and decide each at its prompt.", counts[verifyAwaiting])
			return
		}
		if counts[verifyMissing] > 0 {
			// Missing IS a drift verdict — exit 1 — but --fix is not its
			// remedy; the next update restores it unprompted.
			warn("nothing to mark: the remaining issue is %d missing file(s) — run `ndf update` directly; it restores them without a prompt.", counts[verifyMissing])
			os.Exit(1)
		}
		// Clean project: nothing to fix is success, and the documented
		// chain `ndf verify --fix && ndf update` must reach its second
		// half. Fall through to verifyExitCode(counts, true) → 0.
		ok("nothing to fix — the install record is clean.")
		return
	}

	rawErr("")
	rawErr("--fix marks the drifted file(s) above for re-offer: the next `ndf update` shows each one")
	rawErr("as a diff of your file vs the framework's, and you decide per file. Nothing else changes now.")
	rawErr("Note: the mark is written to the committed install record, so it is visible to your team")
	rawErr("until an update resolves it.")
	choice := strings.ToLower(strings.TrimSpace(prompt("Mark these files for re-offer? [y/N]", "n")))
	if choice != "y" && choice != "yes" {
		warn("nothing changed.")
		os.Exit(1)
	}
	marker.PendingReoffer = dedupSorted(append(marker.PendingReoffer, drifted...))
	if err := writeMarker(marker); err != nil {
		die("write marker: %v", err)
	}
	ok("%d file(s) marked for re-offer. Now run `ndf update` and decide each file at its prompt.", len(drifted))
}

// sprintfCount renders "3 drifted"-style summary fragments.
func sprintfCount(n int, label string) string {
	return fmt.Sprintf("%d %s", n, label)
}
