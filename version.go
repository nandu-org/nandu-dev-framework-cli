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
// Declared as `var` (not `const`) so the release workflow can override it via
// `-ldflags "-X main.CLIVersion=..."` to bake the actual git tag into the
// binary. Local dev builds (no -X flag) get this default value.
var CLIVersion = "2.1.1"

// FrameworkRepo is the GitHub slug of the framework files repo (private).
const FrameworkRepo = "nandu-org/nandu-dev-framework"

// CLIRepo is the GitHub slug of this CLI's repo (public). Used by the team
// handoff message and any self-update logic.
const CLIRepo = "nandu-org/nandu-dev-framework-cli"
