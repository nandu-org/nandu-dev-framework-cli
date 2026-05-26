package main

import (
	"fmt"
	"os"
	"strings"
)

// offerCommitAndPush mirrors bash CLI's _offer_commit_and_push: if we're in
// a git repo and there are uncommitted changes after the update, ask the
// user [Y/n] to commit + push. Default yes — the team handoff message
// printed next assumes the changes are on origin/main.
//
// If the user declines, we print the manual commands they should run before
// pasting the handoff to chat.
//
// migrationCount: see printTeamHandoff's docstring. From cmdUpdate's main
// path, this is always 0 — the gate-fired path returns early before
// reaching here.
func offerCommitAndPush(targetVersion string, changes []change, migrationCount int) {
	projDir := os.Getenv("CLAUDE_PROJECT_DIR")
	if projDir == "" {
		var err error
		projDir, err = os.Getwd()
		if err != nil {
			return
		}
	}
	if !gitIsRepo(projDir) {
		return
	}
	// No-op update + no migration → nothing to commit.
	if len(changes) == 0 && migrationCount == 0 {
		return
	}
	if !gitHasUncommittedChanges(projDir) {
		return
	}

	branch := gitCurrentBranch(projDir)
	if branch != "main" && branch != "master" {
		warn("you are on branch %q, not the typical integration branch.", branch)
		warn("Framework updates usually land on main first; coworkers pull from there.")
	}

	rawErr("")
	rawErr("Framework changes are uncommitted on branch %q.", branch)
	rawErr("Coworkers will pull from main, so push before sharing the team handoff.")
	rawErr("")

	choice := strings.ToLower(strings.TrimSpace(prompt("Commit and push these changes now? [Y/n]", "y")))
	switch choice {
	case "n", "no", "skip":
		warn("Skipped. Before pasting the team handoff into chat:")
		warn("  git -C %q add -A", projDir)
		warn("  git -C %q commit -m \"ndf: update to v%s\"", projDir, targetVersion)
		warn("  git -C %q push origin %s", projDir, branch)
		return
	}

	if _, err := gitInRepo(projDir, "add", "-A"); err != nil {
		warn("git add failed: %v — commit yourself manually before sharing handoff.", err)
		return
	}
	if _, err := gitInRepo(projDir, "commit", "-m", fmt.Sprintf("ndf: update to v%s", targetVersion)); err != nil {
		warn("git commit failed: %v — resolve and commit manually before sharing handoff.", err)
		return
	}
	info("committed: ndf: update to v%s", targetVersion)

	if err := gitInRepoStreaming(projDir, "push", "origin", branch); err != nil {
		warn("git push failed: %v — the commit landed locally but is NOT yet on remote.", err)
		warn("Push manually before sharing the team handoff message.")
		return
	}
	ok("pushed to origin/%s", branch)
}

// migrationHandoffText returns the migration-specific team-handoff text
// the CLI writes to .ndf-pending-handoff on the gate-fired run, for the
// migrator to paste into team chat after /ndf-migrate completes and they
// re-run `ndf update`. Today only v3-to-v4-feature-scoped carries a
// custom message; other migrations return empty and rely on the standard
// printTeamHandoff output.
//
// Why a per-migration message: the v3→v4 reshape moves planning artifacts
// from docs/plan/ to .ndf/, which strands any developer who happens to be
// on a v3-style phase-N/<slug> branch when migration lands on main. The
// standard handoff ("pull main, /compact") doesn't address that — the
// migrator's coworkers need branch-recovery instructions.
//
// Future extensibility: this dispatcher is the known extension point. A
// future migration with custom team-handoff text adds a new case to the
// switch below. Hardcoding the text here keeps the CLI surface small;
// if the set of custom-handoff migrations grows past a handful, this
// is the natural place to factor it out (e.g. into a map or a
// per-migration file shipped via the manifest).
func migrationHandoffText(migrationName string) string {
	switch migrationName {
	case "v3-to-v4-feature-scoped":
		return v3to4TeamHandoffText
	case "v4.0-to-v4.2-heavyweight-phases":
		return v40to42TeamHandoffText
	case "v4.3-to-v4.4-cli-state-relocation":
		return v43to44TeamHandoffText
	}
	return ""
}

const v3to4TeamHandoffText = `====================
TEAM HANDOFF — paste this in your team chat
====================

The framework migrated from v3 to v4. Planning artifacts moved from
docs/plan/ to .ndf/. Your local branches may need to be updated.

If you are on main:
  Run git pull. Your tree now matches the new layout.

If you are on a phase-N/<slug> branch (v3-style):
  1. Commit any uncommitted work on the phase branch
  2. git checkout main && git pull
  3. In Claude Code, run /planning. Describe what you were doing on the
     phase branch; /planning will set up the work in the new
     feature/<NNN>-<slug> layout (typically as a new feature; if the
     work continues an already-shipped phase, mention that so /planning
     can match it to the archived feature). If the recovery path isn't
     obvious, reach out to your Nandu contact before proceeding.

If you are on a v4 feature/* branch:
  Nothing to do — already on the new layout.

If multiple developers had in-flight work on the same phase:
  Coordinate before any of you runs /planning. Feature numbering is
  first-come first-served and the slug locks in at re-creation time. If
  the recovery isn't obvious, reach out to your Nandu contact before
  proceeding.

====================
`

const v40to42TeamHandoffText = `====================
TEAM HANDOFF — paste this in your team chat
====================

The framework upgraded from v4.0.x to v4.2. Phased features now use
per-phase folders for spec/design/tasks instead of ## Phase M: sections
inside shared files. Flat features are unchanged.

If you are on main:
  Run git pull. Your tree now matches the new layout.

If you have uncommitted local edits to a phased feature's feature-level
spec.md / design.md / tasks.md:
  Those files changed shape — phase content moved into
  phase-M-<phase-slug>/ subfolders. Compare your edits against the new
  per-phase folders and relocate them accordingly. The feature-level
  spec.md / design.md now hold cross-phase content only; the feature-level
  tasks.md is gone (tasks live per-phase under phase-M-<phase-slug>/).

If you are on a flat feature branch (feature/<NNN>-<slug>, no phases):
  Nothing to do — flat features are unchanged.

If you are on a phased feature's feature branch:
  That branch is now the integration branch; per-phase work happens on
  per-phase branches feature/<NNN>-<slug>/phase-M-<phase-slug>. When you
  next run /implement from the feature branch, it will surface a
  decision-request to pick a phase, then cut the per-phase branch.

If multiple developers had in-flight phased work:
  The per-phase /planning sub-flow is just-in-time — plan a phase when
  you're ready to implement it. Coordinate before running /planning on
  the same phase. If the recovery isn't obvious, reach out to your Nandu
  contact before proceeding.

====================
`

const v43to44TeamHandoffText = `====================
TEAM HANDOFF — paste this in your team chat
====================

The framework moved CLI-managed state into ` + "`.ndf/cli/`" + `. **Before pulling
main, run ` + "`ndf self-update`" + ` to upgrade to CLI v2.5.0+** — older CLIs
cannot find the marker at its new location, so ` + "`ndf update`" + `,
` + "`ndf config get`" + ` (for marker-sourced keys), and the framework's slash
commands (` + "`/field-note`" + `, ` + "`/ndf-migrate`" + `) will report 'not an ndf project'
until you upgrade. (` + "`ndf version`" + `, ` + "`ndf self-update`" + `, and ` + "`ndf login`" + `
still work without the marker.) After CLI update: ` + "`git pull origin main`" + `,
then ` + "`/compact`" + `. If you have uncommitted local edits to ` + "`.ndf.json`" + `,
` + "`.ndf-pending-*`" + `, or ` + "`.ndf-migrations/`" + ` (rare), reach out before pulling.

====================
`

// printTeamHandoff produces the paste-ready block for team chat after a
// non-no-op update. Skipped entirely if nothing changed.
//
// Format is exactly preserved from the bash CLI — coworkers learn to
// recognize this shape, and we deliberately don't drift.
//
// migrationCount semantics: the number of structural migrations that
// fired on THIS update run (i.e., specs that needed /ndf-migrate to be
// run before file-level changes could land). NOT the total number of
// migrations in the manifest — manifest entries stay forever per the
// maintainer skill's "specs stay in the manifest indefinitely" rule.
// Passing len(manifest.Migrations) here is wrong; it was the v2.1.0 bug
// fixed in v2.1.1. From cmdUpdate's main path, this is always 0 because
// the gate-fired path returns early before reaching here.
func printTeamHandoff(fromV, toV string, changes []change, migrationCount int) {
	if len(changes) == 0 && migrationCount == 0 {
		return
	}
	rawOut("")
	rawOut("====================")
	rawOut("TEAM HANDOFF — paste in your team chat")
	rawOut("====================")
	rawOut("")
	if fromV == toV {
		rawOut("Framework updated on main (drift fixes; version stays at v%s)", toV)
	} else {
		rawOut("Framework updated: v%s → v%s (on main)", fromV, toV)
	}
	rawOut("")

	if migrationCount > 0 {
		rawOut("⚠️  THIS UPDATE INCLUDES A STRUCTURAL MIGRATION.")
		rawOut("   After merging main, run /ndf-migrate in Claude Code, then continue.")
		rawOut("")
	}

	if len(changes) > 0 {
		rawOut("Changed (%d file(s)):", len(changes))
		for _, c := range changes {
			rawOut("- %s (%s)", c.Path, c.Kind)
		}
		rawOut("")
	}

	rawOut("What you need to do:")
	rawOut("- git pull origin main")
	rawOut("- If on a phase branch: git merge main (or rebase per team convention)")
	if migrationCount > 0 {
		rawOut("- Run /ndf-migrate in your Claude Code session")
	}
	rawOut("- If you have an active Claude Code session: /compact after merging")
	rawOut("")
	rawOut("CHANGELOG: https://github.com/%s/blob/main/CHANGELOG.md", FrameworkRepo)
	rawOut("")
	rawOut("====================")
}
