package main

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
//     from a prior `ndf update` is already on disk." Billy's 2026-05-20
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
//     404-tolerant fetcher is gone. Post-Vera the mechanism has no use
//     case — AMVisor will self-author, future canary-shape clients are
//     unbounded and self-author too. The migration spec creates the
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
// Declared as `var` (not `const`) so the release workflow can override it via
// `-ldflags "-X main.CLIVersion=..."` to bake the actual git tag into the
// binary. Local dev builds (no -X flag) get this default value.
var CLIVersion = "2.5.1"

// FrameworkRepo is the GitHub slug of the framework files repo (private).
const FrameworkRepo = "nandu-org/nandu-dev-framework"

// CLIRepo is the GitHub slug of this CLI's repo (public). Used by the team
// handoff message and any self-update logic.
const CLIRepo = "nandu-org/nandu-dev-framework-cli"
