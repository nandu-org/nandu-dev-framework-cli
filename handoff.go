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
