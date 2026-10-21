package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// changeKind classifies an entry in the change log we accumulate during
// update — used by the team handoff message.
type changeKind string

const (
	changeNew                changeKind = "new"
	changeUpdate             changeKind = "update"
	changeUpdateWithConflict changeKind = "update-with-conflict"
	changeRename             changeKind = "rename"
	changeRemove             changeKind = "remove"
)

type change struct {
	Kind changeKind
	Path string // for rename: "old → new"
}

// cmdUpdate is the entry point for `ndf update`.
func cmdUpdate(args []string) {
	var requestedVersion string
	useLatest := false

	for _, a := range args {
		switch {
		case a == "--latest":
			useLatest = true
		case strings.HasPrefix(a, "--version="):
			requestedVersion = strings.TrimPrefix(a, "--version=")
		case a == "-h" || a == "--help":
			printHelpUpdate()
			return
		default:
			die("unknown update flag: %s", a)
		}
	}
	if requestedVersion != "" && useLatest {
		die("--version and --latest are mutually exclusive.")
	}

	// Write command: operate on the project in the current directory. See
	// anchorProjectToCwd — this keeps the marker, framework files, sentinels,
	// and git all resolving to cwd, so `ndf update` can never find the project
	// in one place and write files into another.
	anchorProjectToCwd()

	// Friendly upfront message if no PAT — saves the user from a generic
	// "401 Bad credentials" later in the flow.
	if resolveToken() == "" {
		die("no framework PAT configured.\n\nRun `ndf login` to set your tokens, then re-run `ndf update`.")
	}

	marker := requireMarker()
	currentVersion := marker.Version
	pinnedVersion := ""
	if marker.PinnedVersion != nil {
		pinnedVersion = *marker.PinnedVersion
	}

	// Determine the target version to resolve against the framework repo.
	target := ""
	switch {
	case requestedVersion != "":
		target = requestedVersion
	case useLatest:
		target = "" // means: ask the tags API
	case pinnedVersion != "":
		target = pinnedVersion
	}

	ref, err := resolveRef(target)
	if err != nil {
		die("resolve target ref: %v", err)
	}
	info("fetching manifest for %s…", ref)

	manifest, err := fetchManifest(ref)
	if err != nil {
		die("fetch manifest: %v", err)
	}
	checkMinCLIVersion(manifest)

	if manifest.Version == currentVersion {
		info("already at v%s; checking for drift…", currentVersion)
	} else {
		info("updating from v%s to v%s", currentVersion, manifest.Version)
	}

	// Where this release's /ndf-migrate reads specs — the manifest says
	// (migrations_dir), else the project-root migrations/ folder. Resolved
	// before the gate so a defective value stops the run before any write.
	specDir, err := manifest.specDeliveryDir()
	if err != nil {
		die("%v", err)
	}

	// The change log the team handoff and the commit offer read. Declared
	// before the gate because the applied-migrations branch below can
	// record removals (specs of applied migrations left on disk).
	var changes []change

	// ---- migration gate ----
	// Sentinel-aware: each migration in manifest.Migrations is "applied" iff
	// its sentinel exists (.ndf/cli/sentinels/<name>.complete, or the
	// pre-v2.5.0 .ndf-migrations/ location). If every listed migration is
	// already applied, fall through to the regular file-level update flow.
	// Otherwise, pre-deliver only the pending specs + slash command, write
	// the pending-migration marker so /ndf-migrate knows which to run, and
	// stop here.
	pendingMigrations := pendingMigrationsFromManifest(manifest)
	if len(manifest.Migrations) > 0 && len(pendingMigrations) == 0 {
		info("all %d migration(s) in manifest already applied (sentinels present); skipping migration gate.", len(manifest.Migrations))
		// A pending-migration marker on disk here means a /ndf-migrate run
		// is unfinished: every sentinel exists, but the run stopped between
		// its sentinel write and its commit. /ndf-migrate's own re-run
		// finishes it (Steps 6–7) — and needs this marker and the spec to
		// do so. Keep both, touch nothing, and send the operator there.
		if pendingMigrationExists() {
			warn("a /ndf-migrate run is unfinished in this project (its pending-migration marker is still present).")
			warn("Run /ndf-migrate in Claude Code to finish it, then re-run `ndf update`.")
			return
		}
		// Consume the team-handoff marker FIRST, before any tidy-up.
		// consumePendingHandoff's defense-in-depth check looks for
		// .ndf-pending-migration on disk and bails out if present
		// (signaling that /ndf-migrate's own cleanup didn't complete);
		// it needs to see the pre-tidy state to do its job. If we
		// tidied first the check would always pass and the defense
		// would be inert.
		consumePendingHandoff()
		// Tidy up any stale pending-migration marker from older CLI
		// versions or interrupted /ndf-migrate runs. Dual-path during
		// the catch-up window: a pre-v2.5.0 marker may still be at OLD;
		// both removes succeed-or-no-op via os.IsNotExist tolerance.
		_ = os.Remove(pendingMigrationPath())
		_ = os.Remove(oldPendingMigrationPath())
		// Specs of applied migrations have nothing left to do. Earlier CLIs
		// never removed them (projects committed them into their own
		// migrations/ folder). Clear them from every place they can have
		// been delivered to; tracked ones are recorded as changes.
		tidyAppliedMigrationSpecs(manifest, specDir, &changes)
	} else if len(pendingMigrations) > 0 {
		// Pre-flight: working tree must be clean before we touch it.
		// We're about to write spec files and a marker into the client
		// repo; mixing those with the user's own uncommitted work
		// would force /ndf-migrate's later state-check to disentangle
		// them and risks the user losing changes during the migration
		// commit. Halt early and let the user commit or stash first.
		// This pre-flight used to live in /ndf-migrate.md alone —
		// moved here in v4.0 so we never contaminate a dirty tree in
		// the first place.
		preflightCleanWorkingTreeForMigration()

		info("this release includes %d pending structural migration(s); pre-delivering specs…", len(pendingMigrations))
		for _, name := range pendingMigrations {
			// The framework repo keeps every spec at migrations/<name>.md;
			// the project receives it where the release's /ndf-migrate
			// reads it (specDir): for a framework release whose manifest
			// sets migrations_dir that is the CLI-owned .ndf/cli/migrations/,
			// not a folder the project may own for its own database
			// migrations. A name that is not a plain basename would reach
			// outside that folder — a release defect, stop before writing.
			if !validMigrationName(name) {
				die("manifest migration name %q is not a plain file name", name)
			}
			dest := specDeliveryPath(specDir, name)
			info("  %s", dest)
			if err := fetchFileTo(ref, "migrations/"+name+".md", dest); err != nil {
				die("fetch %s: %v", dest, err)
			}
		}
		// Always re-deliver the slash command; specs may reference its
		// latest behavior.
		info("  .claude/commands/ndf-migrate.md")
		if err := fetchFileTo(ref, ".claude/commands/ndf-migrate.md", ".claude/commands/ndf-migrate.md"); err != nil {
			die("fetch ndf-migrate.md: %v", err)
		}
		// Write the pending-migration marker so /ndf-migrate knows what to apply.
		if err := writePendingMigration([]byte(strings.Join(pendingMigrations, "\n") + "\n")); err != nil {
			die("write %s: %v", pendingMigrationPath(), err)
		}
		// If any pending migration carries migration-specific
		// team-handoff text, stage it for the next `ndf update` run
		// (after /ndf-migrate completes and every listed migration's
		// sentinel is on disk). consumePendingHandoff on the
		// sentinel-skip branch above consumes and removes it.
		composeAndWritePendingHandoff(pendingMigrations)
		ok("")
		ok("v%s includes a structural migration. Run /ndf-migrate in Claude Code to apply,", manifest.Version)
		ok("then re-run `ndf update` to complete the file-level changes.")
		return
	}

	// ---- pass 1: process every file in the new manifest ----
	var customizablePreserved []string
	// Paths the user (or the non-TTY default) skipped at a conflict or
	// collision prompt this run. Feeds ledgerAfterUpdate — a skipped file
	// must NOT be recorded as delivered. skippedDisplay is the user-facing
	// companion: one entry per DECISION (a declined rename contributes two
	// ledger paths but one decision), driving the end-of-run summary.
	var skipped []string
	var skippedDisplay []string
	installed := marker.InstalledChecksums
	// Pending re-offer marks (verify --fix / prior declines). Cleared by
	// omission: only paths a handler explicitly KEEPS this run survive into
	// the rewritten marker. keepPendingMark is the policy: any delivery
	// clears; a TYPED skip on a recorded path clears (a human answered);
	// an unattended auto-skip keeps (a default is not a decision — the
	// release's own incident doctrine); an entry-less net-new decline
	// keeps regardless (the mark is its sole visibility). A path that
	// turned user_customizable or left the manifest clears — moot.
	pendingSet := make(map[string]struct{}, len(marker.PendingReoffer))
	for _, p := range marker.PendingReoffer {
		pendingSet[p] = struct{}{}
	}
	var pendingKept []string

	for _, f := range manifest.Files {
		// user_customizable files (e.g. the pre-commit test hook) are
		// client-owned after scaffolding — never silent-replace them. This
		// path deliberately ignores the marker's installed checksum and
		// compares on-disk content directly against the manifest, so the
		// protection holds even when the marker entry is missing or stale
		// (the field-note scenario that motivated the flag).
		if f.UserCustomizable {
			handleUserCustomizable(ref, f, &changes, &customizablePreserved)
			continue
		}
		if f.RenamedFrom != "" {
			handleRename(ref, f, installed, &changes, &skipped, &skippedDisplay, &pendingKept)
			continue
		}
		if _, hadBefore := installed[f.Path]; !hadBefore {
			// Net-new file — the framework wants to create a path it has
			// never installed here before. The normal case is that nothing
			// occupies that path yet, so we silently create it. But if the
			// client authored their OWN untracked file at this path, a
			// blind fetchFileTo would atomically clobber it (temp+rename
			// overwrites unconditionally). Guard with an os.Stat first.
			if _, err := os.Stat(f.Path); err == nil {
				// Something already exists on disk at the path a net-new
				// framework file wants to create — an untracked-file
				// collision. Never silently overwrite a client's file:
				// hand off to the diff-and-prompt path.
				handleNetNewCollision(ref, f, &changes, &skipped, &skippedDisplay, &pendingKept)
				continue
			}
			info("  new: %s", f.Path)
			if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
				die("fetch %s: %v", f.Path, err)
			}
			changes = append(changes, change{Kind: changeNew, Path: f.Path})
			continue
		}
		// Existing file — route via the pure decision table (the branch
		// order is load-bearing; see updateFileAction).
		installedSha := installed[f.Path]
		_, isPending := pendingSet[f.Path]
		switch updateFileAction(installedSha, sha256OfFileOrEmpty(f.Path), f.Checksum, isPending) {
		case actionNoop:
			// Covers recorded-current-and-untouched AND the client's edit
			// of a recorded-current file (left alone until the next version
			// delta, per the v1.2.2 doctrine: never silently repair).
		case actionRestore:
			warn("  %s: file missing locally; restoring from framework.", f.Path)
			if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
				die("fetch %s: %v", f.Path, err)
			}
			changes = append(changes, change{Kind: changeUpdate, Path: f.Path})
		case actionRecordDelivered:
			// Disk already equals the incoming version — record it, fetch
			// nothing, prompt nothing, report no change. This is the
			// pristine-checkout self-heal for a pulled re-offer mark.
			info("  %s already matches the incoming version; recording it as delivered.", f.Path)
		case actionSilentReplace:
			info("  update: %s", f.Path)
			if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
				die("fetch %s: %v", f.Path, err)
			}
			changes = append(changes, change{Kind: changeUpdate, Path: f.Path})
		case actionConflict:
			handleUpdateConflict(ref, f, installedSha, &changes, &skipped, &skippedDisplay, &pendingKept)
		}
	}

	// ---- compute new_checksums: DELIVERED entries only ----
	// CRITICAL: see marker.writeMarker() docstring, both halves. Never disk
	// state (v1.2.2 — silent customization revert), and never mere intent —
	// a file skipped at a prompt keeps its previous installed checksum, so
	// the next update still sees it as outdated and re-offers it instead of
	// short-circuiting forever on a ledger entry for a delivery that never
	// happened. Preserved user_customizable files join the carry list for
	// the same reason: nothing was delivered to them this run, so their
	// entries keep the last delivered placeholder reference rather than
	// silently advancing to the new manifest's.
	notDelivered := append(append([]string{}, skipped...), customizablePreserved...)
	newChecksums := ledgerAfterUpdate(manifest.Files, installed, notDelivered)

	// ---- pass 2: removed files (in old manifest, not in new, not as a
	// rename source we already handled) ----
	newPaths := manifest.pathSet()
	renameSources := manifest.renameSources()
	for oldPath := range installed {
		if _, kept := newPaths[oldPath]; kept {
			continue
		}
		if _, isRenameSource := renameSources[oldPath]; isRenameSource {
			continue
		}
		if _, err := os.Stat(oldPath); err == nil {
			// Remove only content the framework itself delivered. A file
			// whose on-disk hash differs from its recorded checksum is the
			// client's content — edited by them, or preserved by them at a
			// declined-rename prompt whose carried-forward entry keeps the
			// path in this loop's view. Destroying it on the framework's
			// say-so would undo a decision the client made explicitly;
			// leave it and say so.
			if !shouldRemoveRetiredPath(installed[oldPath], sha256OfFileOrEmpty(oldPath)) {
				warn("%s is no longer in the framework, but your copy differs from what the framework delivered — leaving it in place. Delete it yourself if you no longer want it.", oldPath)
				continue
			}
			warn("removing %s (no longer in framework)", oldPath)
			if err := os.Remove(oldPath); err != nil {
				warn("  could not remove %s: %v", oldPath, err)
			} else {
				changes = append(changes, change{Kind: changeRemove, Path: oldPath})
			}
		}
	}

	// ---- update .ndf.json ----
	// pinning logic mirrors bash CLI:
	//   --version=X   → pin to X
	//   --latest      → clear pin
	//   default       → preserve existing pin
	var newPinned *string
	switch {
	case requestedVersion != "":
		v := manifest.Version
		newPinned = &v
	case useLatest:
		newPinned = nil
	case pinnedVersion != "":
		v := pinnedVersion
		newPinned = &v
	}

	newMarker := &Marker{
		Version:            manifest.Version,
		PinnedVersion:      newPinned,
		InstalledChecksums: newChecksums,
		FieldnotesRepo:     marker.FieldnotesRepo, // preserve through update
		UserCustomizable:   manifest.customizablePaths(),
		PendingReoffer:     dedupSorted(pendingKept),
	}
	if err := writeMarker(newMarker); err != nil {
		die("write marker: %v", err)
	}

	// The completion line must not overclaim: after auto-skips, "complete"
	// would assert a state the run knowingly did not reach. Exit code stays
	// 0 either way — the operation itself succeeded, the undelivered state
	// is legible here and via `ndf verify` (a deliberate contract; changing
	// update's exit semantics would break existing pipelines).
	if len(skippedDisplay) == 0 {
		ok("ndf update complete. Now at v%s.", manifest.Version)
	} else {
		ok("ndf update finished at v%s — %d file(s) not delivered (see below).", manifest.Version, len(skippedDisplay))
	}

	if len(customizablePreserved) > 0 {
		ok("")
		ok("Left your customized file(s) untouched (the framework never overwrites these):")
		for _, p := range customizablePreserved {
			ok("  %s", p)
		}
	}

	// Skipped files are the one outcome that must not scroll away silently:
	// each one is a divergence from the target version that persists until
	// someone decides it. Especially load-bearing when stdin is not a
	// terminal (an agent or CI ran the update): every conflict prompt above
	// auto-answered "skip", and this summary is the only trace.
	if len(skippedDisplay) > 0 {
		warn("")
		if !isStdinTTY() {
			warn("%d file(s) were skipped automatically because no interactive terminal was attached (each line says what that means):", len(skippedDisplay))
		} else {
			warn("%d file(s) were skipped (each line says what that means):", len(skippedDisplay))
		}
		for _, p := range skippedDisplay {
			warn("  %s", p)
		}
		// No blanket outcome claim here — the per-line annotations carry it
		// (a version-delta skip re-surfaces; a typed re-offer decline is
		// final; an undecided one stays marked). The two pointers below are
		// true in every case.
		warn("Run `ndf verify` to see the resulting install state.")
		if !isStdinTTY() {
			warn("Re-run `ndf update` in an interactive terminal to decide the undecided files.")
		}
	}

	// ---- offer commit + push BEFORE the team handoff ----
	// The handoff tells coworkers to `git pull`; if our changes aren't
	// pushed yet, the handoff is premature.
	//
	// migrationCount=0: the migration gate did NOT fire on this run. By
	// the time we reach this line, either there were no migrations in the
	// manifest, or every migration's sentinel was already present (gate
	// skipped). The gate-fired path returns early above. Passing
	// len(manifest.Migrations) here was the v2.1.0 bug — manifest entries
	// stay forever per the maintainer skill, so it always reported a
	// non-zero count and the handoff/commit prompts incorrectly warned
	// about a structural migration on every update past v3.2.0.
	pushed, outstanding := offerCommitAndPush(manifest.Version, changes, 0)

	// ---- team handoff message ----
	printTeamHandoff(os.Stdout, currentVersion, manifest.Version, changes, 0, handoffGitContext(), pushed, outstanding, skippedDisplay)
}

// pendingMigrationsFromManifest walks manifest.Migrations and returns those
// that DON'T yet have a sentinel file written at EITHER the v2.5.0+ path
// (.ndf/cli/sentinels/<name>.complete) OR the pre-v2.5.0 location
// (.ndf-migrations/<name>.complete). The dual-path check makes the gate
// transparent during the catch-up window — stale clients with sentinels
// at OLD don't re-run migrations that already completed.
func pendingMigrationsFromManifest(m *Manifest) []string {
	var out []string
	for _, name := range m.Migrations {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !migrationSentinelExists(name) {
			out = append(out, name)
		}
	}
	return out
}

// handleRename implements the rename branch from bash CLI's pass 1:
//
//   - If the rename source isn't in the installed manifest, treat as new.
//   - If the source file on disk matches its old installed checksum, do a
//     silent rename (download under new path, remove old path).
//   - If the client modified the source file, surface a diff and prompt.
func handleRename(ref string, f ManifestFile, installed map[string]string, changes *[]change, skipped, skippedDisplay, pendingKept *[]string) {
	rn := f.RenamedFrom
	oldInstalledSha, ok := installed[rn]
	if !ok {
		warn("rename %s → %s: source not found in installed manifest. Treating as a new file.", rn, f.Path)
		// Same guard as the net-new branch in pass 1: if something already
		// occupies the target path, never blind-overwrite it — a client's
		// own file (or a drifted file whose record `verify --fix` reset)
		// deserves the diff prompt, not a silent replace.
		if _, err := os.Stat(f.Path); err == nil {
			handleNetNewCollision(ref, f, changes, skipped, skippedDisplay, pendingKept)
			return
		}
		if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
			die("fetch %s: %v", f.Path, err)
		}
		*changes = append(*changes, change{Kind: changeNew, Path: f.Path})
		return
	}

	currentSha := sha256OfFileOrEmpty(rn)
	if currentSha == oldInstalledSha {
		// Client did not modify the source — safe rename.
		info("  rename: %s → %s", rn, f.Path)
		if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
			die("fetch %s: %v", f.Path, err)
		}
		_ = os.Remove(rn)
		*changes = append(*changes, change{Kind: changeRename, Path: rn + " → " + f.Path})
		return
	}

	// Client modified rn — download framework version under new path, prompt.
	// If the client authored their OWN file at the rename TARGET, capture it
	// first: the fetch below overwrites the path, and every prompt outcome
	// must either restore those bytes ([s]) or preserve them as a backup
	// ([r]/[b]) — the "never silently overwrite a client's file" guarantee
	// applies to the target side of a rename exactly as it does to net-new.
	var priorTarget []byte
	hadPriorTarget := false
	if _, err := os.Stat(f.Path); err == nil {
		var readErr error
		priorTarget, readErr = readAll(f.Path)
		if readErr != nil {
			// Fail closed: fetchFileTo overwrites via temp+rename, which
			// succeeds over a file we could not read — proceeding would
			// let every branch act on empty bytes and announce a zero-byte
			// "restore"/"backup" as success. Withdraw the whole rename
			// instead (B4's shape: nothing delivered ⇒ record a skip).
			warn("  rename %s → %s: cannot read the existing file at the target path (%v) — not touching it; rename skipped.", rn, f.Path, readErr)
			*skipped = append(*skipped, rn, f.Path)
			*skippedDisplay = append(*skippedDisplay, rn+" (rename not applied: target unreadable)")
			// An involuntary skip is not a decision — keep any re-offer mark.
			*pendingKept = append(*pendingKept, rn)
			return
		}
		hadPriorTarget = true
		warn("  rename %s → %s: a file already exists at the target path; it will be preserved unless you choose otherwise.", rn, f.Path)
	}
	warn("  rename %s → %s: client has modified %s; downloading framework version to %s for review.", rn, f.Path, rn, f.Path)
	if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
		die("fetch %s: %v", f.Path, err)
	}
	restorePriorTarget := func() {
		if hadPriorTarget {
			if err := os.WriteFile(f.Path, priorTarget, 0o644); err != nil {
				warn("    could not restore your previous file at %s: %v", f.Path, err)
			} else {
				info("    your previous file at %s restored.", f.Path)
			}
		} else {
			_ = os.Remove(f.Path)
		}
	}
	backupPriorTarget := func() {
		if hadPriorTarget {
			if err := os.WriteFile(f.Path+".local-backup", priorTarget, 0o644); err != nil {
				warn("    could not back up your previous file at %s: %v", f.Path, err)
			} else {
				info("    your previous file at %s saved to %s.local-backup.", f.Path, f.Path)
			}
		}
	}
	if isStdinTTY() {
		clientBytes, _ := readAll(rn)
		frameworkBytes, _ := readAll(f.Path)
		rawErr("  diff (yours vs framework, on the new path):")
		printDiff("yours: "+rn, clientBytes, "framework: "+f.Path, frameworkBytes)
	} else {
		warn("  No terminal attached — auto-skipping the rename; re-run `ndf update` interactively to decide.")
	}

	choice := prompt(fmt.Sprintf("  [r]eplace %s's changes with framework version, [s]kip rename, or [b]ackup-and-replace? (default: s)", rn), "s")
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "r":
		backupPriorTarget()
		_ = os.Remove(rn)
		info("    %s removed; framework content lives at %s now.", rn, f.Path)
		*changes = append(*changes, change{Kind: changeRename, Path: rn + " → " + f.Path})
	case "b":
		if err := copyFile(rn, rn+".local-backup"); err == nil {
			backupPriorTarget()
			_ = os.Remove(rn)
			info("    backed up %s → %s.local-backup; framework content at %s.", rn, rn, f.Path)
			*changes = append(*changes, change{Kind: changeRename, Path: rn + " → " + f.Path})
		} else {
			// Backup failed ⇒ nothing was delivered: mirror the skip path so
			// the ledger, the summary, and the working tree all agree (the
			// framework file placed above is withdrawn; a delivered-looking
			// file absent from `changes` would be committed nowhere while
			// the record claimed it).
			warn("    backup failed: %v — leaving %s in place and withdrawing the framework version at %s.", err, rn, f.Path)
			restorePriorTarget()
			*skipped = append(*skipped, rn, f.Path)
			*skippedDisplay = append(*skippedDisplay, rn+" (rename not applied: backup failed)")
			// A failed action is not a decision — keep any re-offer mark.
			*pendingKept = append(*pendingKept, rn)
		}
	default:
		// skip — undo the framework version we just placed. Recording BOTH
		// paths as skipped keeps the source's ledger entry alive (so the
		// rename re-offers next run, diffing against the kept file) and
		// gives the target no entry. An unattended auto-skip keeps any
		// re-offer mark on the source (not a decision).
		restorePriorTarget()
		info("    rename skipped; %s preserved.", rn)
		*skipped = append(*skipped, rn, f.Path)
		*skippedDisplay = append(*skippedDisplay, rn+" (declined rename to "+f.Path+")")
		if keepPendingMark(true, isStdinTTY()) {
			*pendingKept = append(*pendingKept, rn)
		}
	}
}

// fileAction is updateFileAction's verdict for one recorded file.
type fileAction string

const (
	actionNoop            fileAction = "noop"             // recorded current, disk untouched-or-client-edited: nothing to do this run
	actionRestore         fileAction = "restore"          // file vanished locally: re-fetch
	actionRecordDelivered fileAction = "record-delivered" // disk already equals the incoming version: record, no fetch, no prompt
	actionSilentReplace   fileAction = "silent-replace"   // client unchanged since last delivery: fetch + replace
	actionConflict        fileAction = "conflict"         // genuine three-way divergence: diff + prompt
)

// updateFileAction routes a recorded, framework-owned file in pass 1. Pure —
// the branch ORDER is load-bearing and pinned by tests:
//
//   - a vanished file is restored before anything else is considered;
//   - disk already matching the incoming version beats every other branch —
//     re-fetching identical bytes is never wanted, and this is how a
//     pristine checkout self-heals a pulled re-offer mark with no prompt,
//     no change entry, and no network;
//   - a ledger that claims current (installed == manifest) is a no-op
//     UNLESS the path carries a re-offer mark — the mark exists precisely
//     to overrule that short-circuit (a client-edited recorded-current file
//     stays untouched until the next version delta, per the v1.2.2
//     doctrine: never silently repair content);
//   - client-unchanged since last delivery → silent replace;
//   - anything else is a real conflict → diff + prompt.
func updateFileAction(installedSha, currentSha, manifestSha string, isPending bool) fileAction {
	switch {
	case currentSha == "":
		return actionRestore
	case currentSha == manifestSha:
		if installedSha == manifestSha && !isPending {
			return actionNoop
		}
		return actionRecordDelivered
	case installedSha == manifestSha && !isPending:
		return actionNoop
	case currentSha == installedSha:
		return actionSilentReplace
	default:
		return actionConflict
	}
}

// conflictSkipOutcome renders the per-file message and the summary-line
// suffix for a conflict skip. decided = a human typed the skip; a non-TTY
// auto-answer and a failed [r]/[b] action (the human chose replace and the
// delivery failed) are NOT decisions and keep the re-offer mark. The three
// truths, per case:
//
//	re-offer + decided  → final: not offered again automatically
//	re-offer + !decided → stays marked; re-offered on the next interactive run
//	version delta       → re-surfaces on the next update (the carried entry
//	                      differs from the manifest, so the conflict re-fires)
//
// Pure — unit-tested.
func conflictSkipOutcome(reoffer, decided bool) (fileMsg, displaySuffix string) {
	switch {
	case reoffer && decided:
		return "your file is preserved. It will not be offered again automatically — `ndf verify` shows it as drifted, and `ndf verify --fix` re-opens it.",
			" (declined; not offered again — `ndf verify --fix` re-opens it)"
	case reoffer && !decided:
		return "your file is preserved and stays marked for re-offer.",
			" (not decided; stays marked for re-offer)"
	default:
		return "your file is preserved and this will re-surface on the next update.",
			" (skipped; re-offered on the next update)"
	}
}

// keepPendingMark decides whether a skip outcome preserves a path's
// re-offer mark. A TYPED skip on a recorded path is a human decision — the
// question was answered, the mark clears (the file stays visible as
// drifted). An UNATTENDED auto-skip is not a decision (the release's own
// incident doctrine), so the mark survives and the promised interactive
// re-run genuinely re-prompts. An entry-less path (declined net-new
// collision) keeps its mark regardless: the mark is its only visibility.
// Pure — unit-tested.
func keepPendingMark(hadEntry, typedDecision bool) bool {
	return !hadEntry || !typedDecision
}

// dedupSorted returns the sorted unique elements of in. Used for marker
// list fields so rewrites are stable and idempotent.
func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, p := range in {
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// shouldRemoveRetiredPath decides whether pass 2 may delete a file the
// manifest no longer tracks: only when its on-disk content is exactly what
// the framework last delivered. Anything else is the client's content —
// edited by them, or kept at a declined-rename prompt — and destroying it
// on the framework's say-so would undo an explicit client decision. Pure —
// unit-tested in update_test.go.
func shouldRemoveRetiredPath(recorded, actual string) bool {
	return actual == recorded
}

// ledgerAfterUpdate computes the marker's installed_checksums after a pass-1
// run. The rule (marker.writeMarker docstring, both halves): the ledger
// records what the framework last DELIVERED — never disk state, never mere
// intent. Concretely:
//
//   - delivered or already-current file → the manifest checksum
//   - file skipped at a conflict prompt → its PREVIOUS installed checksum,
//     carried forward, so the next update still sees manifest != installed
//     and re-offers it (instead of short-circuiting forever on an entry for
//     a delivery that never happened)
//   - net-new file skipped at the collision prompt → no entry (stays "new"
//     next run, so the collision prompt re-fires)
//   - rename skipped → the SOURCE path's entry carries forward (the rename
//     re-offers next run, diffing against the client's kept file) and the
//     target path gets no entry
//   - preserved user_customizable file → previous entry carries forward
//     (nothing was delivered; the entry keeps the last delivered
//     placeholder reference instead of silently advancing)
//
// notDelivered is the union of prompt-skipped paths and preserved
// customizable paths. Pure — unit-tested in update_test.go.
func ledgerAfterUpdate(files []ManifestFile, prev map[string]string, notDelivered []string) map[string]string {
	inManifest := make(map[string]struct{}, len(files))
	for _, f := range files {
		inManifest[f.Path] = struct{}{}
	}
	skip := make(map[string]struct{}, len(notDelivered))
	for _, p := range notDelivered {
		skip[p] = struct{}{}
	}
	out := make(map[string]string, len(files))
	for _, f := range files {
		if _, wasSkipped := skip[f.Path]; wasSkipped {
			if prevSha, had := prev[f.Path]; had {
				out[f.Path] = prevSha
			}
			continue
		}
		out[f.Path] = f.Checksum
	}
	// Not-delivered paths outside the manifest's files[] are rename sources
	// the user kept: carry their entries forward so the rename re-offers.
	for _, p := range notDelivered {
		if _, m := inManifest[p]; m {
			continue
		}
		if prevSha, had := prev[p]; had {
			out[p] = prevSha
		}
	}
	return out
}

// handleUpdateConflict handles pass 1's actionConflict verdict: the file
// differs from both the last delivered version and the incoming one (or
// was marked for re-offer). Diff + prompt; a skip is appended to *skipped
// so the ledger and the end-of-run summary record it, and the re-offer
// mark survives per keepPendingMark (a typed skip is a decision; an
// unattended auto-skip is not).
func handleUpdateConflict(ref string, f ManifestFile, installedSha string, changes *[]change, skipped, skippedDisplay, pendingKept *[]string) {
	tmp := f.Path + ".ndf-incoming"
	if err := fetchFileTo(ref, f.Path, tmp); err != nil {
		die("fetch %s: %v", f.Path, err)
	}
	defer os.Remove(tmp)

	// Two arrivals look identical here except for the ledger: a genuine
	// version delta (installed != manifest), and a re-offer of a file the
	// framework hasn't changed (installed == manifest, forced by the mark).
	// The wording must not claim a newer framework version that does not
	// exist.
	reoffer := installedSha == f.Checksum
	if reoffer {
		warn("  %s: differs from the version the framework last delivered here (the framework's copy is unchanged — this file was marked for re-offer).", f.Path)
	} else {
		warn("  %s: differs from the last version the framework delivered here, and the framework has a newer version.", f.Path)
	}
	if isStdinTTY() {
		warn("  If you edited this file, review the diff. If you never edited it, an earlier update likely skipped it — choose [r].")
		clientBytes, _ := readAll(f.Path)
		frameworkBytes, _ := readAll(tmp)
		printDiff("yours: "+f.Path, clientBytes, "framework: "+f.Path, frameworkBytes)
	} else {
		// No terminal: the prompt below auto-answers skip. Don't instruct a
		// reader to choose at a prompt that will never appear, and don't
		// dump a diff nobody can act on — the end-of-run summary names the
		// file and the interactive re-run is the route (the re-offer mark
		// survives an auto-skip, so that re-run genuinely re-prompts).
		warn("  No terminal attached — auto-skipping. If you did not edit this file, re-run `ndf update` in an interactive terminal and choose [r].")
	}

	// decided = a human typed this skip. The failure branches pass false:
	// the human answered REPLACE there, and a failed action is not a
	// decision — the mark must survive so the file stays reachable.
	skipHere := func(decided bool) {
		fileMsg, suffix := conflictSkipOutcome(reoffer, decided)
		info("    skipped %s; %s", f.Path, fileMsg)
		*skipped = append(*skipped, f.Path)
		*skippedDisplay = append(*skippedDisplay, f.Path+suffix)
		if keepPendingMark(true, decided) {
			*pendingKept = append(*pendingKept, f.Path)
		}
	}

	choice := prompt("  [r]eplace with framework, [s]kip, or [b]ackup-and-replace? (default: s)", "s")
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "r":
		if err := copyFile(tmp, f.Path); err != nil {
			warn("    replace failed: %v", err)
			skipHere(false)
		} else {
			info("    replaced %s.", f.Path)
			*changes = append(*changes, change{Kind: changeUpdateWithConflict, Path: f.Path})
		}
	case "b":
		if err := copyFile(f.Path, f.Path+".local-backup"); err == nil {
			if err := copyFile(tmp, f.Path); err == nil {
				info("    backed up %s → %s.local-backup; replaced with framework version.", f.Path, f.Path)
				*changes = append(*changes, change{Kind: changeUpdateWithConflict, Path: f.Path})
			} else {
				warn("    replace failed after backup: %v", err)
				skipHere(false)
			}
		} else {
			warn("    backup failed: %v — not replacing.", err)
			skipHere(false)
		}
	default:
		skipHere(isStdinTTY())
	}
}

// handleNetNewCollision handles the case where a net-new framework file
// (one the framework has never installed in this project before) wants to
// create a path that is ALREADY occupied by a file on disk — an untracked
// file the client authored themselves. The "never silently overwrite a
// client's file" guarantee forbids the blind silent-create the net-new
// branch would otherwise do, so we surface a diff and prompt instead.
//
// Wording note: this is deliberately NOT handleUpdateConflict's "differs from"
// locally and upstream" message. The framework never installed anything
// here, so there is no upstream change relative to a prior install — there
// is simply a pre-existing untracked file sitting where the framework now
// wants to put a brand-new file. The prompt reflects that accurately.
//
// Reuses handleUpdateConflict's temp-file + printDiff + [r]/[s]/[b] machinery, with
// skip as the default — when in doubt we preserve the client's file. A skip
// is appended to *skipped: the file gets NO ledger entry, so it stays
// net-new and this prompt re-fires on the next update.
func handleNetNewCollision(ref string, f ManifestFile, changes *[]change, skipped, skippedDisplay, pendingKept *[]string) {
	if sha256OfFileOrEmpty(f.Path) == f.Checksum {
		// The client's file is byte-identical to what the framework wants
		// to install — record it as delivered, no prompt. Also how a
		// pulled re-offer mark on a healthy checkout clears itself.
		info("  %s already matches the framework's version; recording it as installed.", f.Path)
		return
	}
	tmp := f.Path + ".ndf-incoming"
	if err := fetchFileTo(ref, f.Path, tmp); err != nil {
		die("fetch %s: %v", f.Path, err)
	}
	defer os.Remove(tmp)

	warn("  %s: a file already exists here that the framework did not install; not overwriting it.", f.Path)
	if isStdinTTY() {
		existingBytes, _ := readAll(f.Path)
		frameworkBytes, _ := readAll(tmp)
		printDiff("yours (existing): "+f.Path, existingBytes, "framework (new): "+f.Path, frameworkBytes)
	} else {
		warn("  No terminal attached — auto-skipping; your file is preserved. Re-run `ndf update` interactively to decide.")
	}

	choice := prompt("  [r]eplace your file with the framework version, [s]kip, or [b]ackup-and-replace? (default: s)", "s")
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "r":
		if err := copyFile(tmp, f.Path); err != nil {
			warn("    replace failed: %v", err)
			*skipped = append(*skipped, f.Path)
			*skippedDisplay = append(*skippedDisplay, f.Path+" (replace failed; your file kept)")
			*pendingKept = append(*pendingKept, f.Path) // entry-less: the mark is its only visibility
		} else {
			info("    replaced %s with the framework version.", f.Path)
			*changes = append(*changes, change{Kind: changeNew, Path: f.Path})
		}
	case "b":
		if err := copyFile(f.Path, f.Path+".local-backup"); err == nil {
			if err := copyFile(tmp, f.Path); err == nil {
				info("    backed up %s → %s.local-backup; installed the framework version.", f.Path, f.Path)
				*changes = append(*changes, change{Kind: changeNew, Path: f.Path})
			} else {
				warn("    replace failed after backup: %v", err)
				*skipped = append(*skipped, f.Path)
				*skippedDisplay = append(*skippedDisplay, f.Path+" (replace failed; your file kept)")
				*pendingKept = append(*pendingKept, f.Path)
			}
		} else {
			warn("    backup failed: %v — not replacing.", err)
			*skipped = append(*skipped, f.Path)
			*skippedDisplay = append(*skippedDisplay, f.Path+" (backup failed; your file kept)")
			*pendingKept = append(*pendingKept, f.Path)
		}
	default:
		info("    skipped %s; your existing file is preserved.", f.Path)
		*skipped = append(*skipped, f.Path)
		*skippedDisplay = append(*skippedDisplay, f.Path+" (your existing file kept; the framework"+"’"+"s version was not installed)")
		// An entry-less decline has no ledger entry to keep it visible —
		// the pending mark is its only trace in `ndf verify`/`ndf version`.
		*pendingKept = append(*pendingKept, f.Path)
	}
}

// customizableAction decides what `update` should do with a user_customizable
// file, given its current on-disk checksum (empty = absent) and the manifest
// checksum. Pure + marker-independent on purpose — see handleUserCustomizable.
//
//	"" (absent on disk)            -> "create"   (deliver the placeholder)
//	== manifestChecksum (matches)  -> "skip"     (nothing to do)
//	anything else (client-owned)   -> "preserve" (never overwrite)
func customizableAction(currentChecksum, manifestChecksum string) string {
	switch currentChecksum {
	case "":
		return "create"
	case manifestChecksum:
		return "skip"
	default:
		return "preserve"
	}
}

// handleUserCustomizable processes a manifest file flagged user_customizable.
// The framework scaffolds these once (e.g. .claude/hooks/pre-commit-tests.sh,
// a placeholder the client replaces with their real test command) but never
// owns them afterward. Unlike the conflict path, the decision here does NOT consult
// the marker's installed checksum: it compares on-disk content directly against
// the manifest. That makes the "don't clobber my customization" guarantee
// robust even when the marker's installed_checksums entry is missing or stale —
// the exact field-note failure (a multi-version update across the
// .ndf.json -> .ndf/cli/install.json relocation left the entry absent, and the
// pre-Go bash CLI then treated the customized file as net-new and overwrote it).
func handleUserCustomizable(ref string, f ManifestFile, changes *[]change, preserved *[]string) {
	switch customizableAction(sha256OfFileOrEmpty(f.Path), f.Checksum) {
	case "create":
		info("  new (customizable): %s", f.Path)
		if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
			die("fetch %s: %v", f.Path, err)
		}
		*changes = append(*changes, change{Kind: changeNew, Path: f.Path})
	case "skip":
		// On disk already matches the framework placeholder; nothing to do.
	default: // "preserve"
		// Client has customized it (on disk differs from the manifest).
		// Never silent-replace; surface it in the post-update summary.
		*preserved = append(*preserved, f.Path)
	}
}

// copyFile copies src to dst, creating dst's parent directory as needed.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// preflightCleanWorkingTreeForMigration halts cmdUpdate with a clear,
// non-framework-terminology message if the working tree has uncommitted
// changes. Fires only on the gate-fired branch (before any framework file
// has been touched). Halt is fatal — caller does not return.
//
// Failure-mode discipline: if we can't determine the working tree state
// (cwd lookup failed, git is on disk but errors), we halt with a clear
// message rather than silently letting the migration delivery contaminate
// a possibly-dirty tree. The one exception is "not a git repo at all":
// non-git projects have nothing to preflight against and proceed.
//
// Re-invocation: re-firing the gate on a subsequent `ndf update` is
// idempotent — the spec gets re-fetched (overwriting the prior copy)
// and the .ndf-pending-migration marker gets re-written. v2.2.0 had a
// short-circuit here that died with "A migration delivery from a prior
// `ndf update` is already on disk." That short-circuit was removed in
// v2.3.1 (a 2026-05-20 field-note scenario): the message was
// misleading when the project's identity tag changed after a prior
// gate-fired delivery, and re-firing the gate is safer than refusing
// to do so — it always lands the correct, current spec for THIS run.
func preflightCleanWorkingTreeForMigration() {
	projDir := os.Getenv("CLAUDE_PROJECT_DIR")
	if projDir == "" {
		var err error
		projDir, err = os.Getwd()
		if err != nil {
			die("cannot determine project directory: %v", err)
		}
	}
	if !gitIsRepo(projDir) {
		return // non-git project; nothing to check
	}
	out, err := gitInRepo(projDir, "status", "--porcelain")
	if err != nil {
		die("could not verify clean working tree before migration: %v\n\nFix the underlying git issue, then re-run `ndf update`.", err)
	}
	if out == "" {
		return
	}
	die("This release includes a structural migration, and ndf update needs a clean working tree before delivering the migration spec.\n\nUncommitted changes:\n%s\n\nPlease commit or stash them, then re-run `ndf update`.", indentEachLine(out, "  "))
}

// indentEachLine prefixes each line of s with prefix. Trailing empty
// lines are dropped.
func indentEachLine(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// writePendingMigration writes the pending-migration marker, creating
// the parent directory (.ndf/cli/) first under the v2.5.0+ layout.
// Called from the gate-fired branch of cmdUpdate. Refactored out of the
// inline call site for symmetry with writePendingHandoff and to
// guarantee the MkdirAll precondition is never forgotten by a future
// caller.
func writePendingMigration(body []byte) error {
	if err := os.MkdirAll(filepath.Dir(pendingMigrationPath()), 0o755); err != nil {
		return fmt.Errorf("create pending-migration parent dir: %w", err)
	}
	return os.WriteFile(pendingMigrationPath(), body, 0o644)
}

// writePendingHandoff writes the pending-handoff marker to its v2.5.0+
// location, creating .ndf/cli/ first if needed. Low-level helper —
// callers that want the compose-then-write shape (concatenate per-spec
// handoff text from the dispatcher) use composeAndWritePendingHandoff.
func writePendingHandoff(body []byte) error {
	if err := os.MkdirAll(filepath.Dir(pendingHandoffPath()), 0o755); err != nil {
		return fmt.Errorf("create pending-handoff parent dir: %w", err)
	}
	return os.WriteFile(pendingHandoffPath(), body, 0o644)
}

// composeAndWritePendingHandoff writes the migration team-handoff marker
// if any of the pending migrations carry custom handoff text. When
// multiple migrations have text, they're concatenated in pending-order
// separated by a blank line. Empty text → no marker file written.
//
// Consumed by consumePendingHandoff on the sentinel-skip branch of the
// NEXT cmdUpdate run (after the migrator has run /ndf-migrate and every
// listed migration's sentinel is on disk).
//
// Called from the gate-fired branch AFTER spec delivery — the migration
// hasn't run yet at this point, only its spec has been pre-delivered.
func composeAndWritePendingHandoff(pendingMigrations []string) {
	var parts []string
	for _, name := range pendingMigrations {
		if t := migrationHandoffText(name); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return
	}
	body := strings.Join(parts, "\n")
	if err := writePendingHandoff([]byte(body)); err != nil {
		// Non-fatal: the spec has been delivered and pending-migration
		// marker is in place; failing to stage the team-handoff text
		// only means the user will see the standard handoff (no
		// migration-specific branch-recovery instructions) on the
		// post-migration `ndf update` re-run.
		warn("could not write %s: %v — migration team-handoff will not be available on next `ndf update`.", pendingHandoffPath(), err)
	}
}

// loadPendingHandoff reads the pending-handoff marker, dual-path aware:
// tries the v2.5.0+ path first, falls back to the pre-v2.5.0 location.
// Returns (body, "new" | "old" | "", error). nil body + no error if
// neither path exists.
func loadPendingHandoff() ([]byte, string, error) {
	if data, err := os.ReadFile(pendingHandoffPath()); err == nil {
		return data, "new", nil
	} else if !os.IsNotExist(err) {
		return nil, "", err
	}
	if data, err := os.ReadFile(oldPendingHandoffPath()); err == nil {
		return data, "old", nil
	} else if !os.IsNotExist(err) {
		return nil, "", err
	}
	return nil, "", nil
}

// consumePendingHandoff prints the pending-handoff marker (if present)
// and removes it. Called from the sentinel-skip branch — i.e., only when
// every manifest-listed migration is already applied — so the message we
// print is always correct (the migration HAS completed).
//
// Defense-in-depth: even though the call site already gates on "all
// migrations applied", we additionally check that no pending-migration
// marker exists at EITHER NEW or OLD path (the in-flight signal that
// /ndf-migrate removes on success). If still on disk, the migration
// didn't actually complete — skip quietly and leave the handoff text for
// the next run.
//
// Dual-path cleanup: removes the handoff marker from BOTH NEW and OLD
// locations on success; both os.Remove calls succeed-or-no-op via
// os.IsNotExist tolerance. Covers the catch-up window where a
// pre-v4.4.0 client wrote the handoff at OLD before upgrading.
//
// No-op if the marker is absent from both locations.
func consumePendingHandoff() {
	if pendingMigrationExists() {
		return // migration still pending; not safe to declare it completed
	}
	data, _, err := loadPendingHandoff()
	if err != nil || data == nil {
		return // absent or unreadable; nothing to do
	}
	rawOut("")
	rawOut("%s", strings.TrimRight(string(data), "\n"))
	rawOut("")
	// Remove from both possible locations — both calls succeed-or-no-op
	// for the absent path via os.IsNotExist.
	if err := os.Remove(pendingHandoffPath()); err != nil && !os.IsNotExist(err) {
		warn("could not remove %s: %v — the handoff message above may print again on the next `ndf update`.", pendingHandoffPath(), err)
	}
	if err := os.Remove(oldPendingHandoffPath()); err != nil && !os.IsNotExist(err) {
		warn("could not remove %s: %v — the handoff message above may print again on the next `ndf update`.", oldPendingHandoffPath(), err)
	}
}
