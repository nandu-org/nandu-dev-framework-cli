package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// offerCommitAndPush: if we're in a git repo and there are uncommitted
// changes after the update, offer to commit + push them so the team handoff
// message (which tells coworkers to pull) isn't premature.
//
// Consent rules — the operator is often an agent inside a coding session,
// not a human at a terminal, and there every default becomes an unattended
// decision:
//
//   - stdin not a TTY → NEVER run a state-changing git command (read-only
//     probes like status/rev-parse/check-ignore still run). Print the
//     manual commands and return. An unattended `ndf update` must not be
//     able to commit or push anything (a 2026-07 field incident: a session-run update
//     auto-answered the old [Y/n]-default-yes prompt and pushed to the
//     client's integration branch against their explicit instruction).
//   - interactive → ask, DEFAULT NO. Pushing is the kind of action that
//     requires an explicit yes, not an absence of objection.
//   - staging is scoped to this run's own changes + .ndf/cli/ — never
//     `git add -A`, which would sweep unrelated working-tree changes into
//     a commit labeled as a framework update.
//
// The non-main warning stays informational: a client's integration branch
// may legitimately be develop; with default-no the push there is opt-in.
//
// migrationCount: see printTeamHandoff's docstring. From cmdUpdate's main
// path, this is always 0 — the gate-fired path returns early before
// reaching here.
//
// Returns (pushed, outstanding): pushed = the changes reached the remote;
// outstanding = uncommitted changes exist that a coworker would need.
// printTeamHandoff needs both — a handoff that tells coworkers to pull
// local-only changes is one dishonesty; a NOT-PUSHED banner on a run whose
// restores left the tree byte-identical to the committed state is the
// opposite one (alarming about nothing).
func offerCommitAndPush(targetVersion string, changes []change, migrationCount int) (pushed, outstanding bool) {
	projDir := os.Getenv("CLAUDE_PROJECT_DIR")
	if projDir == "" {
		var err error
		projDir, err = os.Getwd()
		if err != nil {
			return false, false
		}
	}
	if !gitIsRepo(projDir) {
		return false, false
	}
	// No-op update + no migration → nothing to commit.
	if len(changes) == 0 && migrationCount == 0 {
		return false, false
	}
	stage := stagePathsForCommit(changes)
	// Scoped to the update's own paths: unrelated WIP elsewhere in the
	// tree must neither trigger the offer nor arm the NOT-PUSHED banner,
	// and a restore that left everything byte-identical needs no commit.
	if !gitHasUncommittedChangesIn(projDir, stage) {
		return false, false
	}

	branch := gitCurrentBranch(projDir)

	printManual := func(reason string) {
		warn("%s", reason)
		// The printed pathspec must be runnable as-is: a gitignored
		// .ndf/cli in the main `git add` makes the whole command exit
		// non-zero (version-dependently half-staging), and git's own hint
		// ("use -f") invites the wrong fix — so emit it as its own
		// optional line instead when the project ignores it. Paths are
		// quoted so a future path with spaces cannot split.
		mainStage := make([]string, 0, len(stage))
		var ignoredStage []string
		cliState := false
		for _, sp := range stage {
			if sp == ".ndf/cli" {
				cliState = true // printed as its own line below, with its exclude
				continue
			}
			if _, err := gitInRepo(projDir, "check-ignore", "-q", sp); err == nil {
				ignoredStage = append(ignoredStage, sp)
				continue
			}
			mainStage = append(mainStage, sp)
		}
		quoted := make([]string, len(mainStage))
		for i, sp := range mainStage {
			quoted[i] = fmt.Sprintf("%q", sp)
		}
		if len(mainStage) > 0 {
			warn("  git -C %q add -- %s", projDir, strings.Join(quoted, " "))
		}
		if cliState {
			cs := cliStatePathspec()
			if _, err := gitInRepo(projDir, "check-ignore", "-q", ".ndf/cli"); err == nil {
				ignoredStage = append(ignoredStage, ".ndf/cli")
			} else {
				warn("  git -C %q add -- %q %q", projDir, cs[0], cs[1])
			}
		}
		for _, sp := range ignoredStage {
			warn("  # %s is gitignored in this project — include it only if your team commits it:", sp)
			warn("  # git -C %q add -f -- %q", projDir, sp)
		}
		warn("  git -C %q commit -m \"ndf: update to v%s\"", projDir, targetVersion)
		if branch == "" {
			warn("  # detached HEAD — no branch to push to; check out a branch before pushing.")
		} else {
			warn("  git -C %q push origin %s", projDir, branch)
		}
	}

	// The non-main caution prints on BOTH paths. On the unattended path it
	// matters most: nobody is watching the branch there, and the manual
	// push command below names it — it must not read as a plain to-do.
	if branch == "" {
		warn("detached HEAD — there is no branch to push to; check out a branch before sharing these changes.")
	} else if !onIntegrationBranch(branch) {
		warn("you are on branch %q, not the typical integration branch.", branch)
		warn("Framework updates usually land on main first; coworkers pull from there.")
	}

	if !isStdinTTY() {
		rawErr("")
		// Phrased as a report, not a task: on this path the reader is
		// usually an agent, and an imperative "To commit: <commands>"
		// reads as an instruction to execute — the incident shape this
		// release removes. The consent line is unconditional.
		printManual("Not committed or pushed (no interactive terminal). If these changes should be shared, the commands are below — do not run them without your operator's explicit go-ahead:")
		if branch != "" && !onIntegrationBranch(branch) {
			warn("Note: the push command above targets %q — confirm with your operator that framework updates belong there.", branch)
		}
		return false, true
	}

	if branch == "" {
		printManual("Not committing (detached HEAD).")
		return false, true
	}

	rawErr("")
	rawErr("Framework changes are uncommitted on branch %q.", branch)
	rawErr("Coworkers pull them from %s once you push, so push before sharing the team handoff.", branch)
	rawErr("")

	choice := prompt("Commit and push these changes now? [y/N]", "n")
	if !commitPushConsented(choice) {
		printManual("Not committing.")
		return false, true
	}

	// Stage per-path, tolerating individual failures: with an explicit
	// pathspec list, one path git cannot match (a removed file that was
	// never tracked, a gitignored .ndf/) must not abort the commit the
	// user just approved — nor leave a half-staged index behind, which is
	// what a single multi-path `git add` does when it fails mid-list.
	staged := 0
	for _, p := range stage {
		args := []string{"add", "--", p}
		if p == ".ndf/cli" {
			args = append([]string{"add", "--"}, cliStatePathspec()...)
		}
		if _, err := gitInRepo(projDir, args...); err != nil {
			warn("  could not stage %s: %v — continuing with the rest.", p, err)
			continue
		}
		staged++
	}
	if staged == 0 {
		warn("nothing could be staged — commit yourself manually before sharing the handoff.")
		return false, true
	}
	if _, err := gitInRepo(projDir, "commit", "-m", fmt.Sprintf("ndf: update to v%s", targetVersion)); err != nil {
		warn("git commit failed: %v — resolve and commit manually before sharing handoff.", err)
		return false, true
	}
	info("committed: ndf: update to v%s", targetVersion)

	if err := gitInRepoStreaming(projDir, "push", "origin", branch); err != nil {
		warn("git push failed: %v — the commit landed locally but is NOT yet on remote.", err)
		warn("Push manually before sharing the team handoff message.")
		return false, true
	}
	ok("pushed to origin/%s", branch)
	return true, false
}

// onIntegrationBranch reports whether branch is one the team-handoff flow
// treats as the typical integration branch. Pure — unit-tested.
func onIntegrationBranch(branch string) bool {
	return branch == "main" || branch == "master"
}

// commitPushConsented interprets the commit-and-push prompt answer: only an
// explicit yes consents. Everything else — empty (the default), n, garbage —
// declines. Pure — unit-tested, because this default IS the incident guard:
// the pre-v2.9.0 prompt defaulted to yes, and an unattended `ndf update`
// (where prompts auto-answer their default) committed and pushed with
// nobody deciding.
func commitPushConsented(choice string) bool {
	c := strings.ToLower(strings.TrimSpace(choice))
	return c == "y" || c == "yes"
}

// stagePathsForCommit derives the exact path set to stage for the
// post-update commit: every path this run touched (for renames, both
// sides) plus the CLI-state subtree (marker, sentinels, consumed
// transients). Deliberately NOT `git add -A`: unrelated working-tree
// changes must never ride along in a commit labeled as a framework
// update. Pure — unit-tested in handoff_test.go.
func stagePathsForCommit(changes []change) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, c := range changes {
		if c.Kind == changeRename {
			// Path is "old → new"; stage both sides.
			parts := strings.SplitN(c.Path, " → ", 2)
			for _, p := range parts {
				add(strings.TrimSpace(p))
			}
			continue
		}
		add(c.Path)
	}
	// Marker, sentinels, and consumed transient files all live here. It is
	// staged through cliStatePathspec, which keeps the transient spec
	// folder out — see that function.
	add(".ndf/cli")
	return out
}

// cliStatePathspec is how the `.ndf/cli` stage entry is handed to git: the
// folder plus an exclude for the transient spec folder, so a pending or
// stale spec there is never committed by the framework's own commit. git
// applies an exclude to every positive pathspec in the same command, so
// this pair always goes in a `git add` of its own, never merged into the
// line that stages the update's explicit paths.
func cliStatePathspec() []string {
	return []string{".ndf/cli", ":(exclude)" + cliMigrationSpecsDir}
}

// migrationHandoffText returns the migration-specific team-handoff text
// the CLI writes to .ndf-pending-handoff on the gate-fired run, for the
// migrator to paste into team chat after /ndf-migrate completes and they
// re-run `ndf update`. Migrations without a case return empty and rely on
// the standard printTeamHandoff output.
//
// A missing case is silent and easy to miss: composeAndWritePendingHandoff
// only writes the marker when at least one pending migration returns
// non-empty, so a migration with no case here produces no pending-handoff
// file at all, and the migrator pastes the standard "drift fixes" message.
// Adding the case is the whole content of a release — see v2.3.2, v2.5.0
// and v2.8.1. When a framework release ships a migration, check this switch.
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
	case "v4.15-to-v4.16-settings-split":
		return v415to416TeamHandoffText
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
// handoffGitContext resolves the branch context for the handoff block.
// Empty string when there is no usable branch: detached HEAD, or not a git
// repository at all (anchorProjectToCwd already ran, so cwd is the project).
func handoffGitContext() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if !gitIsRepo(cwd) {
		return ""
	}
	return gitCurrentBranch(cwd)
}

func printTeamHandoff(w io.Writer, fromV, toV string, changes []change, migrationCount int, branch string, pushed, outstanding bool, notDelivered []string) {
	if len(changes) == 0 && migrationCount == 0 {
		return
	}
	// The block is formatted for forwarding to humans, which makes it the
	// one artifact of the run that must not overclaim OR under-claim: it
	// names the branch the changes are actually on (hardcoding main
	// misrouted every coworker of a develop-based team); it carries a
	// NOT-PUSHED banner only while committable changes actually exist
	// locally (a restore that left the tree byte-identical needs no
	// alarm); it lists undelivered files inside the fence so the caveat
	// survives the copy-paste; and with no branch to name (detached HEAD,
	// non-git project) it declines to print a shareable block at all
	// rather than inventing "main".
	if branch == "" {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "(No team handoff printed: there is no git branch to share these changes")
		fmt.Fprintln(w, " from. If this project uses git, check out your integration branch, commit")
		fmt.Fprintln(w, " and push, then re-run `ndf update` for a paste-ready handoff block.)")
		return
	}
	out := func(format string, args ...any) {
		fmt.Fprintf(w, "%s\n", fmt.Sprintf(format, args...))
	}
	out("")
	out("====================")
	out("TEAM HANDOFF — paste in your team chat")
	out("====================")
	out("")
	if !pushed && outstanding {
		out("⚠️  NOT YET PUSHED — these changes are local only. Do not paste this")
		out("   until they are committed and pushed to origin/%s.", branch)
		out("")
	}
	if fromV == toV {
		out("Framework updated on %s (drift fixes; version stays at v%s)", branch, toV)
	} else {
		out("Framework updated: v%s → v%s (on %s)", fromV, toV, branch)
	}
	out("")

	if migrationCount > 0 {
		out("⚠️  THIS UPDATE INCLUDES A STRUCTURAL MIGRATION.")
		out("   After merging %s, run /ndf-migrate in Claude Code, then continue.", branch)
		out("")
	}

	if len(changes) > 0 {
		out("Changed (%d file(s)):", len(changes))
		for _, c := range changes {
			out("- %s (%s)", c.Path, c.Kind)
		}
		out("")
	}

	if len(notDelivered) > 0 {
		out("Not delivered on this run (%d file(s); each line says what that means):", len(notDelivered))
		for _, p := range notDelivered {
			out("- %s", p)
		}
		out("")
	}

	out("What you need to do:")
	out("- git pull origin %s", branch)
	out("- If on a phase branch: git merge %s (or rebase per team convention)", branch)
	if migrationCount > 0 {
		out("- Run /ndf-migrate in your Claude Code session")
	}
	out("- If you have an active Claude Code session: /compact after merging")
	out("")
	out("CHANGELOG: https://github.com/%s/blob/main/CHANGELOG.md", FrameworkRepo)
	out("")
	out("====================")
}

// v415to416TeamHandoffText covers framework v4.16.0's settings.json split.
//
// The one thing this text exists to say is `/clear`, not `/compact`. The
// standard handoff's closing advice is "/compact after merging", which is right
// for every release that only changes prompt text — and wrong for this one:
// v4.16.0 moves the hook logic out of settings.json into .claude/hooks/*.sh, and
// Claude Code reads hooks at session START. /compact does not re-read them, so a
// coworker who follows the standard advice keeps running the pre-split hooks
// until they happen to restart.
//
// Without a case here, composeAndWritePendingHandoff writes no pending-handoff
// at all and coworkers get the standard message. Same shape as the v2.3.2 and
// v2.5.0 dispatcher-case releases.
const v415to416TeamHandoffText = `====================
TEAM HANDOFF — paste this in your team chat
====================

The framework's hooks moved out of ` + "`.claude/settings.json`" + ` into their own
files under ` + "`.claude/hooks/`" + `. ` + "`settings.json`" + ` now holds your permissions plus
one line registering each hook, and it is yours from here on — the framework
will not overwrite it again. Hook fixes now reach you on their own.

After merging: ` + "`/clear`" + ` your Claude Code session — **not** ` + "`/compact`" + `.
Hooks are read at session start, and ` + "`/compact`" + ` does not re-read them, so
` + "`/compact`" + ` leaves you running the old hooks.

**Expect gates you were not seeing before.** Work in a git worktree was running
unguarded: the commit gate ran your *main* checkout's tests rather than the
worktree's, and the branch/task-file and planning-mode gates did not fire there
at all. All three now resolve the worktree you are actually in. A commit from a
worktree runs that worktree's tests and can block; source writes there need a
feature branch and a matching task file.

**If you had extended the plan-check's source directories**, that setting does
not carry over — it used to live inside the hook's own code, which the framework
now owns and replaces. The migration tells the migrator what it was. Set it in
` + "`.claude/hooks/hooks.config.sh`" + ` as ` + "`NDF_SOURCE_ROOTS='src|tests|…'`" + `.
Until then, plan-check watches the framework's defaults instead of your
directories.

====================
`
