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
// Declared as `var` (not `const`) so the release workflow can override it via
// `-ldflags "-X main.CLIVersion=..."` to bake the actual git tag into the
// binary. Local dev builds (no -X flag) get this default value.
var CLIVersion = "2.3.1"

// FrameworkRepo is the GitHub slug of the framework files repo (private).
const FrameworkRepo = "nandu-org/nandu-dev-framework"

// CLIRepo is the GitHub slug of this CLI's repo (public). Used by the team
// handoff message and any self-update logic.
const CLIRepo = "nandu-org/nandu-dev-framework-cli"
