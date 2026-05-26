package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// changeKind classifies an entry in the change log we accumulate during
// update — used by the team handoff message.
type changeKind string

const (
	changeNew                 changeKind = "new"
	changeUpdate              changeKind = "update"
	changeUpdateWithConflict  changeKind = "update-with-conflict"
	changeRename              changeKind = "rename"
	changeRemove              changeKind = "remove"
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

	// ---- migration gate ----
	// Sentinel-aware: each migration in manifest.Migrations is "applied" iff
	// `.ndf-migrations/<name>.complete` exists. If every listed migration is
	// already applied, fall through to the regular file-level update flow.
	// Otherwise, pre-deliver only the pending specs + slash command, write
	// the pending-migration marker so /ndf-migrate knows which to run, and
	// stop here.
	pendingMigrations := pendingMigrationsFromManifest(manifest)
	if len(manifest.Migrations) > 0 && len(pendingMigrations) == 0 {
		info("all %d migration(s) in manifest already applied (sentinels present); skipping migration gate.", len(manifest.Migrations))
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
			specPath := "migrations/" + name + ".md"
			info("  %s", specPath)
			if err := fetchFileTo(ref, specPath, specPath); err != nil {
				die("fetch %s: %v", specPath, err)
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
	var changes []change
	installed := marker.InstalledChecksums

	for _, f := range manifest.Files {
		if f.RenamedFrom != "" {
			handleRename(ref, f, installed, &changes)
			continue
		}
		if _, hadBefore := installed[f.Path]; !hadBefore {
			// Net-new file.
			info("  new: %s", f.Path)
			if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
				die("fetch %s: %v", f.Path, err)
			}
			changes = append(changes, change{Kind: changeNew, Path: f.Path})
			continue
		}
		// Existing file — diff manifest vs installed.
		installedSha := installed[f.Path]
		if installedSha == f.Checksum {
			// Framework hasn't changed it; nothing to do this run.
			continue
		}
		handleUpdate(ref, f, installedSha, &changes)
	}

	// ---- compute new_checksums from the MANIFEST (not from disk) ----
	// CRITICAL: see marker.writeMarker() docstring. Recording disk state
	// here would cause the v1.2.2 corruption bug to come back.
	newChecksums := make(map[string]string, len(manifest.Files))
	for _, f := range manifest.Files {
		newChecksums[f.Path] = f.Checksum
	}

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
	}
	if err := writeMarker(newMarker); err != nil {
		die("write marker: %v", err)
	}

	ok("ndf update complete. Now at v%s.", manifest.Version)

	// ---- offer commit + push BEFORE the team handoff ----
	// The handoff tells coworkers to `git pull`; if our changes aren't
	// pushed yet, the handoff is premature. The bash CLI added this in
	// v1.2.1; we preserve the same prompt-default-yes behavior.
	//
	// migrationCount=0: the migration gate did NOT fire on this run. By
	// the time we reach this line, either there were no migrations in the
	// manifest, or every migration's sentinel was already present (gate
	// skipped). The gate-fired path returns early above. Passing
	// len(manifest.Migrations) here was the v2.1.0 bug — manifest entries
	// stay forever per the maintainer skill, so it always reported a
	// non-zero count and the handoff/commit prompts incorrectly warned
	// about a structural migration on every update past v3.2.0.
	offerCommitAndPush(manifest.Version, changes, 0)

	// ---- team handoff message ----
	printTeamHandoff(currentVersion, manifest.Version, changes, 0)
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
func handleRename(ref string, f ManifestFile, installed map[string]string, changes *[]change) {
	rn := f.RenamedFrom
	oldInstalledSha, ok := installed[rn]
	if !ok {
		warn("rename %s → %s: source not found in installed manifest. Treating as a new file.", rn, f.Path)
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
	warn("  rename %s → %s: client has modified %s; downloading framework version to %s for review.", rn, f.Path, rn, f.Path)
	if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
		die("fetch %s: %v", f.Path, err)
	}
	clientBytes, _ := readAll(rn)
	frameworkBytes, _ := readAll(f.Path)
	rawErr("  diff (yours vs framework, on the new path):")
	printDiff("yours: "+rn, clientBytes, "framework: "+f.Path, frameworkBytes)

	choice := prompt(fmt.Sprintf("  [r]eplace %s's changes with framework version, [s]kip rename, or [b]ackup-and-replace? (default: s)", rn), "s")
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "r":
		_ = os.Remove(rn)
		info("    %s removed; framework content lives at %s now.", rn, f.Path)
	case "b":
		if err := copyFile(rn, rn+".local-backup"); err == nil {
			_ = os.Remove(rn)
			info("    backed up %s → %s.local-backup; framework content at %s.", rn, rn, f.Path)
		} else {
			warn("    backup failed: %v — leaving %s in place.", err, rn)
		}
	default:
		// skip — undo the framework version we just placed
		_ = os.Remove(f.Path)
		info("    rename skipped; %s preserved (framework version at %s removed).", rn, f.Path)
	}
}

// handleUpdate implements the "existing file with new content" branch from
// pass 1: silent replace if client hasn't modified, otherwise diff + prompt.
func handleUpdate(ref string, f ManifestFile, installedSha string, changes *[]change) {
	currentSha := sha256OfFileOrEmpty(f.Path)
	if currentSha == "" {
		// Vanished locally — restore.
		warn("  %s: file missing locally; restoring from framework.", f.Path)
		if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
			die("fetch %s: %v", f.Path, err)
		}
		return
	}
	if currentSha == installedSha {
		// Client unchanged; silent replace.
		info("  update: %s", f.Path)
		if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
			die("fetch %s: %v", f.Path, err)
		}
		*changes = append(*changes, change{Kind: changeUpdate, Path: f.Path})
		return
	}

	// Client AND framework both changed it — conflict prompt.
	tmp := f.Path + ".ndf-incoming"
	if err := fetchFileTo(ref, f.Path, tmp); err != nil {
		die("fetch %s: %v", f.Path, err)
	}
	defer os.Remove(tmp)

	warn("  %s: changed both locally and upstream.", f.Path)
	clientBytes, _ := readAll(f.Path)
	frameworkBytes, _ := readAll(tmp)
	printDiff("yours: "+f.Path, clientBytes, "framework: "+f.Path, frameworkBytes)

	choice := prompt("  [r]eplace with framework, [s]kip, or [b]ackup-and-replace? (default: s)", "s")
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "r":
		if err := copyFile(tmp, f.Path); err != nil {
			warn("    replace failed: %v", err)
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
			}
		} else {
			warn("    backup failed: %v — not replacing.", err)
		}
	default:
		info("    skipped %s.", f.Path)
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
// v2.3.1 (Billy's 2026-05-20 field-note scenario): the message was
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
