package main

import "fmt"

// CLIVersion is the version of the ndf CLI binary itself.
//
// Versioning policy (from the ndf-maintainer skill):
//   - patch  — bug fix, error-message improvement, no behavior change
//   - minor  — new capability or new flag (backwards compatible)
//   - major  — implementation rewrite or breaking change
//
// v2.0.0 — full rewrite from bash to Go. Behaviorally compatible with v1.3.x
// (same commands, flags, file formats, manifest protocol). Drops every runtime
// dependency the bash CLI carried (bash, jq, awk, sed, diff, sha256sum) in
// favor of Go stdlib. Adds native Windows support.
//
// v2.2.0 — companion-file delivery for migration specs (canary maps
// pre-delivered alongside the spec), uncommitted-state pre-flight on the
// gate-fired update path, migration team-handoff marker mechanism
// (.ndf-pending-handoff) for v3→v4-style messages that need to print on the
// post-/ndf-migrate re-run, and a project-identity field in the Marker
// schema. Framework v4.0.0 / v4.0.1 requires this CLI. v4.0.2 bumps
// `min_cli_version` to `2.3.1` to ensure clients have the
// post-companion-delivery semantics (self-authored canary maps, no
// project-identity field, no preflight short-circuit). (Note:
// companion-file delivery for migration specs was retired in v2.3.1 — the
// canary map is now self-authored at /ndf-migrate time. The team-handoff
// marker mechanism is preserved.)
//
// v2.3.0 — `ndf self-update` subcommand: channel-aware print-instructions
// for updating the CLI binary itself (Homebrew, Scoop, install.sh, install.ps1).
// Distinguishes from `ndf update` (which updates framework files in a project).
// `self-update` (not `upgrade`) is the chosen verb because the dominant
// Unix package-manager convention puts `upgrade` on actual binary replacement
// — which this command deliberately does NOT do (package-manager state stays
// authoritative). The `self-` prefix matches pnpm's pattern and removes all
// brew/apt verb confusion.
//
// v2.3.1 — three coordinated removals, paired with framework v4.0.2:
//
//   - Drop the preflight short-circuit that died "A migration delivery
//     from a prior `ndf update` is already on disk." A 2026-05-20
//     field note surfaced the failure mode: when the project's identity
//     tag changed after a prior gate-fired delivery, the short-circuit's
//     recovery message ("run /ndf-migrate") pointed at the wrong action
//     — the user needed to re-fire the gate to pick up the correct,
//     current spec. Re-firing the gate is now idempotent and always
//     safe; letting it happen is better than the misleading halt.
//
//   - Retire companion-file delivery for migration specs entirely. The
//     v2.2.0 mechanism that pre-delivered project-keyed canary maps and
//     optional YAML companions alongside each spec is removed: the
//     gate-fired branch no longer fetches anything but the spec itself,
//     the pending-migration-files directory constant is gone, and the
//     404-tolerant fetcher is gone. The mechanism has no remaining use
//     case — the one migration that consumed pre-delivered maps has
//     completed, and canary-shape projects now self-author their maps
//     at /ndf-migrate time. The migration spec creates the
//     pending-migration-files directory itself via `mkdir -p` if it
//     needs to write a self-authored map; the CLI no longer creates or
//     clears that directory.
//
//   - Strip the project-identity field from Marker. Without
//     companion-file routing the field has no consumer. Old on-disk
//     markers carrying it are tolerated on read (JSON ignores unknown
//     fields) and the field drops off on the next rewrite.
//
// v2.3.2 — paired with framework v4.2.0. Adds the team-handoff dispatcher
// case for the `v4.0-to-v4.2-heavyweight-phases` migration, so coworkers
// running `ndf update` post-migration see a paste-ready handoff message
// covering the artifact-tree change (phased features now use per-phase
// folders for spec/design/tasks). Purely additive — `migrationHandoffText`
// gains one switch case; no schema, flag, or behavior change for any other
// path. `min_cli_version` in framework v4.2.0 stays at `2.3.1`; clients
// still on v2.3.1 get the migration but fall through to the default empty
// handoff text (the standard "pull main + /compact" block from
// printTeamHandoff). Upgrading the CLI is recommended but not required.
//
// v2.4.0 — CLI-as-contract release. Three new read-only subcommands mediate
// external access to the project marker (.ndf.json) so consumers (slash
// commands, hooks, third-party tooling) no longer need to hit the file
// directly:
//
//   - `ndf is-project` — exit 0 if cwd (or $CLAUDE_PROJECT_DIR) contains a
//     parseable .ndf.json, 1 if absent, 2 on internal error. Silent on 0
//     and 1. Replaces the `test -f .ndf.json` idiom.
//
//   - `ndf marker-path` — print the absolute resolved marker path the CLI
//     would consult. Does not check existence; pair with `ndf is-project`
//     if needed.
//
//   - `ndf config get <key> [--source]` — print a single config value to
//     stdout. Closed key set: version, pinned_version, fieldnotes_repo.
//     Accepts both kebab-case (fieldnotes-repo) and snake_case
//     (fieldnotes_repo) via internal normalization. The --source flag
//     prints the resolution source ("marker" or "legacy-config") to
//     stderr. PATs deliberately NOT exposed here — use `ndf config show`
//     for the masked view.
//
// Also refactors `markerPath()` to finally honor $CLAUDE_PROJECT_DIR (the
// pre-existing comment claimed this; the implementation did not). The
// resolver now returns an absolute path rooted at $CLAUDE_PROJECT_DIR (or
// cwd if unset). All existing callers pick this up automatically. The
// writeMarker temp file now lands next to the marker rather than in cwd.
//
// Exit-code convention for the new read-only mediated reads: 0 = success,
// 1 = absent (only `is-project` uses this), 2 = internal error (stderr
// message plus an `ndf:internal-error` stdout marker for environments that
// swallow stderr).
//
// Pure-additive — no flag, schema, behavior, or output change on any other
// code path. `cmdConfigShow` rendering is byte-for-byte preserved under
// existing inputs except for one prose-only update to the legacy-config
// annotation; a golden-file check in scripts/verify-show.sh enforces no
// other drift. No manifest schema change. No `min_cli_version` bump on
// any shipped framework — the framework-side migration to the new
// subcommands ships separately in framework v4.3.0 (which bumps
// `min_cli_version` to `2.4.0` once this CLI has propagated). Existing
// scripts that read `.ndf.json` directly continue to work.
//
// v2.5.0 — paired with framework v4.4.0 (ships after CLI v2.5.0
// propagates through Homebrew, Scoop, and the install scripts).
//
// Release 3 of the CLI-as-contract project: with the contract for
// runtime reads established in v2.4.0 + framework v4.3.0, the marker
// file and its `.ndf-*` siblings are consolidated under `.ndf/cli/`:
// `.ndf/cli/install.json` (was `.ndf.json` at the project root),
// `.ndf/cli/sentinels/` (was `.ndf-migrations/`),
// `.ndf/cli/pending-migration` and `.ndf/cli/pending-handoff` (were
// `.ndf-pending-*` at the project root).
//
// Backwards-compatible read of pre-relocation marker layout during the
// migration window: loadMarker, migrationSentinelExists,
// pendingMigrationExists, pendingHandoffExists, and
// loadPendingHandoff each consult both the new (.ndf/cli/) and old
// (project-root) locations so a stale client whose on-disk state
// hasn't yet been moved by the framework v4.4.0 migration still has
// every CLI subcommand work transparently. Writes (writeMarker,
// writePendingMigration, writePendingHandoff,
// migrationSentinelPath) always land at the new location and carry
// the MkdirAll precondition so .ndf/cli/ is created on demand. The
// `oldMarkerPath`, `oldPendingMigrationPath`, `oldPendingHandoffPath`,
// `oldMigrationSentinelPath` helpers expose the legacy resolver
// shape for the dual-path code paths.
//
// `cmdInit` now refuses on EITHER the new or the old marker existing
// (covers fresh init in a project that hasn't yet run the
// v4.3-to-v4.4 migration) with a message pointing the user at
// `ndf update`.
//
// `migrationHandoffText` gains a case for
// `v4.3-to-v4.4-cli-state-relocation`: the coworker-facing recovery
// instruction emitted by `ndf update` on the post-migration re-run.
//
// Verification: new `scripts/verify-dual-path.sh` runs five fixture
// scenarios covering the catch-up window (marker at OLD only,
// sentinels at OLD only, pending-handoff at OLD only, fresh write at
// NEW, read-OLD-then-write-NEW). Added to `RELEASE.md` pre-flight
// alongside the existing golden-file check.
//
// v2.5.1 — bug fix: `ndf update` no longer silently overwrites an untracked
// file when creating a net-new framework file. The per-file loop's net-new
// branch previously called fetchFileTo unconditionally (atomic temp+rename,
// which overwrites without checking), so a client who had authored their own
// file at the exact path a brand-new framework file targets would have it
// clobbered with no warning. The branch now os.Stat's the destination first:
// if a file already exists there, it falls to a diff-and-prompt
// (handleNetNewCollision) showing "your existing file" vs "the framework's
// new file" with a [r]eplace / [s]kip / [b]ackup-and-replace choice
// (defaulting to skip); the normal empty-path case keeps the silent-create
// behavior. Surfaces with framework v4.7.0, which adds the first net-new
// agent file in a while (`.claude/agents/acceptance-verifier.md`). No
// manifest-format change; no `min_cli_version` bump.
//
// v2.5.2 — maintenance: the `ndf init` field-notes-repo prompt and the
// repo-slug validation error now illustrate the OWNER/REPO shape with a
// neutral placeholder (`nandu-org/Example-FieldNotes`); golden-file
// fixtures updated in lockstep. Comment-level cleanups in config.go,
// update.go, and this file. No flag, schema, or behavior change beyond the two example
// strings; no manifest-format change; no `min_cli_version` bump.
//
// v2.6.0 — defense-in-depth for client-customized framework files. A new
// optional manifest field, `user_customizable: true`, marks files the
// framework scaffolds once but the client owns thereafter (currently only
// `.claude/hooks/pre-commit-tests.sh`, the placeholder a client replaces with
// their real test command). `ndf update` routes such files through
// handleUserCustomizable, whose decision is deliberately marker-INDEPENDENT:
// it compares on-disk content directly against the manifest checksum (absent
// -> create the placeholder; matches -> no-op; differs -> preserve, never
// overwrite) and lists any preserved file in a post-update summary. This
// hardens the "don't clobber my customization" guarantee against a missing or
// stale marker entry — the field-note scenario where a multi-version update
// across the .ndf.json -> .ndf/cli/install.json relocation could leave the
// installed_checksums entry absent. (The net-new collision guard added in
// v2.5.1 already protected the live Go CLI against silent overwrite; this
// makes the protection explicit and independent of marker state.) Surfaces
// with framework v4.7.3, which flags pre-commit-tests.sh. Optional field —
// older CLIs ignore it (JSON tolerates unknown keys) and continue to skip the
// unchanged placeholder via the installed==manifest path, so no
// `min_cli_version` bump.
//
// [Corrected at framework v4.16.0 — the reasoning above is preserved as the
// claim of record and it is SCOPED, not general. "Continue to skip the
// UNCHANGED placeholder" is the whole load-bearing word: it holds only while a
// flagged file's content does not change. Framework v4.16.0 changed the content
// of both flagged files, which destroys the installed==manifest short-circuit
// and routes them into handleUpdate on a CLI that ignores the flag. v4.16.0's
// floor is therefore 2.6.0 — the version that implements UserCustomizable — and
// the general rule is: min_cli_version tracks any guarantee the manifest makes
// that only the CLI can keep, not just manifest format. See manifest.go's
// UserCustomizable field for the full derivation.]
//
// v2.7.0 — `ndf version` now reports the installed framework version too. The
// prior command printed only the CLI binary version; a user standing in a
// folder with an NDF install reasonably expects to also see which framework
// version is installed there (the single most common thing "what version am I
// on?" means in an NDF project). The command now reads the project marker (the
// same read `ndf config get version` performs) and, when a marker is present,
// prints a second line: `framework v<X.Y.Z>` (plus a `(pinned: v<X.Y.Z>)`
// annotation when pinned_version is set).
//
// Deliberate boundaries:
//   - Line 1 stays byte-identical (`ndf v<CLIVersion>`), so the CLI-version
//     read that RELEASE.md smoke checks, README, and `self-update`'s "verify:
//     ndf version" hint all rely on is unchanged. The framework line is
//     additive and only appears inside an NDF project.
//   - The framework line is a HUMAN-facing convenience, not a machine contract.
//     Programmatic reads of the framework version stay on the CLI-as-contract
//     surface (`ndf config get version`); nothing should parse `ndf version`
//     for the framework value. The formatting is intentionally prose ("framework
//     v4.15.0"), not a bare version, to discourage that.
//   - A malformed/unreadable marker degrades gracefully: `ndf version` still
//     prints the CLI line and exits 0, emitting a stderr warning rather than
//     dying. `ndf version` must never become a hard failure just because a
//     project marker is corrupt.
//   - `ndf version --help` (and `-h`) now prints command help instead of
//     silently ignoring the flag and printing the version — closing a small
//     consistency gap with every other subcommand. `ndf --version` / `ndf -v`
//     keep printing the version (no args to interpret).
//
// No framework version bump — framework files are untouched. No manifest-format
// change; no `min_cli_version` bump (older CLIs simply keep printing the CLI
// line only). Minor bump (2.6.0 → 2.7.0): new capability on an existing command.
//
// v2.8.0 — init/update anchor to cwd; fixes the framework-file path split-brain.
// `ndf init` and `ndf update` now resolve the project entirely from the current
// working directory and no longer follow $CLAUDE_PROJECT_DIR (implemented as a
// one-line anchorProjectToCwd() that clears the override in-process at command
// entry). `ndf config set` is deliberately NOT changed — it writes no framework
// files (only the marker, read+written through the same override-aware resolver),
// so it has no split-brain and stays consistent with the read side.
//
// The bug: the marker read/write, sentinels, and git operations honored
// $CLAUDE_PROJECT_DIR, but the framework-file operations (fetch/stat/remove/diff/
// backup, plus init's CLAUDE.project.md write) used bare cwd-relative paths. When
// $CLAUDE_PROJECT_DIR pointed at a real project root that differed from cwd (a
// subfolder run under Claude Code), `ndf update` located the marker in one
// directory while writing framework files into another — a split-brain that
// records checksums the files don't match. Latent (the normal path is a plain
// terminal from the project root, where $CLAUDE_PROJECT_DIR is unset), but a real
// correctness hazard in the safety-critical update file loop.
//
// The fix — cwd-only for writes, not chdir-to-override. The read subcommands
// (is-project, marker-path, config get, config show, version) still honor
// $CLAUDE_PROJECT_DIR because slash commands and hooks invoke them from an
// arbitrary cwd and need the override to find the project. The write commands are
// developer-run from the project they intend to change, so anchoring everything
// to cwd is both simpler and matches the mental model "operate on the project I'm
// standing in." A write command run from outside a project now simply finds no
// marker and refuses ("not an ndf project"), instead of acting on one directory
// while writing into another. Rejected alternative: chdir into $CLAUDE_PROJECT_DIR
// (would make writes work from any subfolder, but re-introduces a "which project
// did you mean?" ambiguity when cwd sits in a different project, needing a guard);
// cwd-only has no such ambiguity because it never consults the override.
//
// Also: `ndf config show` now prints the marker's resolved absolute path (honoring
// $CLAUDE_PROJECT_DIR, since it's a read command) instead of a misleading
// "./…"/"in cwd" that implied the current directory even when the override pointed
// elsewhere. Golden fixtures + verify-show.sh updated to normalize the volatile
// project path.
//
// No framework version bump — framework files are untouched. No manifest-format
// change; no `min_cli_version` bump (older CLIs keep their prior resolution). Minor
// bump (2.7.0 → 2.8.0): it changes the project-resolution behavior of the two
// write commands that place framework files (init/update), so it's surfaced as
// minor even though it's corrective.
//
// v2.8.1 — paired with framework v4.16.0. Adds the team-handoff dispatcher case
// for the `v4.15-to-v4.16-settings-split` migration. Same shape and same reason
// as v2.3.2's case for v4.0-to-v4.2: without an entry, migrationHandoffText
// returns "", composeAndWritePendingHandoff writes no pending-handoff marker at
// all, and the migrator pastes the STANDARD message into team chat.
//
// Why that mattered enough to cut a release for it: the standard message's
// closing advice is "/compact after merging". Framework v4.16.0 moves the hook
// logic out of settings.json into .claude/hooks/*.sh, and Claude Code reads
// hooks at session START — /compact does not re-read them. So every coworker
// following the standard handoff would keep running the pre-split hooks until
// they happened to restart. The new text says /clear, and covers the two other
// things a coworker needs: the gates now fire in git worktrees (where they
// previously did not fire at all), and a project that had extended plan-check's
// source roots must re-set them in hooks.config.sh.
//
// The message is ONE-SHOT: it is generated at migration time, so this CLI has to
// be in a client's hands BEFORE they run the v4.16.0 update. There is no second
// migration to re-message on.
//
// Also corrects the `user_customizable` / `min_cli_version` comments here and on
// manifest.go's field: they said adding the field never forces a floor bump,
// which is true of ADDING it and false as a general rule — the reasoning was
// scoped to a flagged file whose content does not change, and framework v4.16.0
// changed both. Comment-only; no behavior change.
//
// Patch bump (2.8.0 → 2.8.1): a dispatcher entry is data for a capability that
// already exists, not a new one, and the rest is documentation. Precedent:
// v2.3.2, which was the same change for a different migration. No framework
// version bump; no manifest-format change; no `min_cli_version` bump.
//
// Declared as `var` (not `const`) so the release workflow can override it via
// `-ldflags "-X main.CLIVersion=..."` to bake the actual git tag into the
// binary. Local dev builds (no -X flag) get this default value.
var CLIVersion = "2.8.1"

// FrameworkRepo is the GitHub slug of the framework files repo (private).
const FrameworkRepo = "nandu-org/nandu-dev-framework"

// CLIRepo is the GitHub slug of this CLI's repo (public). Used by the team
// handoff message and any self-update logic.
const CLIRepo = "nandu-org/nandu-dev-framework-cli"

// cmdVersion handles `ndf version` (and the `--version` / `-v` aliases routed
// here from main). It prints the CLI binary version and, when the cwd (or
// $CLAUDE_PROJECT_DIR) is an NDF project, the installed framework version.
//
// A malformed marker is non-fatal: the CLI line still prints and we warn to
// stderr rather than dying, so `ndf version` stays a reliable "what am I
// running?" command even against a corrupt project. All formatting lives in the
// pure versionOutput/versionLines helpers; cmdVersion only does the I/O.
func cmdVersion(args []string) {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			printHelpVersion()
			return
		}
	}
	m, err := loadMarker()
	stdout, stderrWarn := versionOutput(CLIVersion, m, err)
	for _, line := range stdout {
		fmt.Println(line)
	}
	if stderrWarn != "" {
		warn("%s", stderrWarn)
	}
}

// versionOutput computes the `ndf version` output from the CLI version and the
// result of loading the project marker. Pure — no I/O — so both the happy path
// and the corrupt-marker degradation are unit-testable. Returns the stdout
// lines plus, when the marker could not be read, a non-empty stderr warning.
//
// On a load error it still returns the CLI line — via versionLines(.., nil), so
// line 1 has a single source of truth — because `ndf version` must never
// hard-fail just because a project marker is corrupt.
func versionOutput(cliVersion string, m *Marker, loadErr error) (stdout []string, stderrWarn string) {
	if loadErr != nil {
		return versionLines(cliVersion, nil),
			fmt.Sprintf("could not read project marker for the framework version: %v", loadErr)
	}
	return versionLines(cliVersion, m), ""
}

// versionLines renders `ndf version` output as a slice of stdout lines, given
// the CLI version and the project marker (nil when not in an NDF project).
// Pure — no I/O — so the format is unit-testable without spawning a process.
//
// Line 1 is always `ndf v<cliVersion>` (kept byte-stable for consumers that
// read the CLI version). When a marker is present, line 2 reports the framework
// version, with a `(pinned: v<X.Y.Z>)` suffix when the project pins a version.
func versionLines(cliVersion string, m *Marker) []string {
	lines := []string{"ndf v" + cliVersion}
	if m == nil {
		return lines
	}
	fw := "(unknown)"
	if m.Version != "" {
		fw = "v" + m.Version
	}
	line := "framework " + fw
	if m.PinnedVersion != nil && *m.PinnedVersion != "" {
		line += " (pinned: v" + *m.PinnedVersion + ")"
	}
	return append(lines, line)
}
