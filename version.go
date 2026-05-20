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
// v2.2.0 — companion-file delivery for migration specs (canary maps land
// under .ndf-pending-migration-files/), uncommitted-state pre-flight on the
// gate-fired update path, migration team-handoff marker mechanism
// (.ndf-pending-handoff) for v3→v4-style messages that need to print on the
// post-/ndf-migrate re-run, and project_tag field in Marker schema. v4.0
// framework requires this CLI.
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
// Declared as `var` (not `const`) so the release workflow can override it via
// `-ldflags "-X main.CLIVersion=..."` to bake the actual git tag into the
// binary. Local dev builds (no -X flag) get this default value.
var CLIVersion = "2.3.0"

// FrameworkRepo is the GitHub slug of the framework files repo (private).
const FrameworkRepo = "nandu-org/nandu-dev-framework"

// CLIRepo is the GitHub slug of this CLI's repo (public). Used by the team
// handoff message and any self-update logic.
const CLIRepo = "nandu-org/nandu-dev-framework-cli"
