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
		// Tidy up any stale pending-migration marker from older CLI versions.
		_ = os.Remove(pendingMigrationPath())
	} else if len(pendingMigrations) > 0 {
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
		if err := os.WriteFile(pendingMigrationPath(), []byte(strings.Join(pendingMigrations, "\n")+"\n"), 0o644); err != nil {
			die("write %s: %v", pendingMigrationPath(), err)
		}
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
	offerCommitAndPush(manifest.Version, changes, len(manifest.Migrations))

	// ---- team handoff message ----
	printTeamHandoff(currentVersion, manifest.Version, changes, len(manifest.Migrations))
}

// pendingMigrationsFromManifest walks manifest.Migrations and returns those
// that DON'T yet have a sentinel file written.
func pendingMigrationsFromManifest(m *Manifest) []string {
	var out []string
	for _, name := range m.Migrations {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := os.Stat(migrationSentinelPath(name)); os.IsNotExist(err) {
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
