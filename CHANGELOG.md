# Changelog

## v2.3.1 — 2026-05-20

**Paired with framework v4.0.2.** Retires the migration-spec companion-file mechanism and removes a stale `ndf update` preflight check.

### What's new

- **Companion-file delivery for migration specs retired.** The `migrations/<name>.<project_tag>.map.yml` companion mechanism introduced in v2.2.0 is removed. The v4.0.2 framework's migration spec now self-authors the canary map at `/ndf-migrate` time by reading the project's own `docs/plan/` state, so the CLI no longer pre-fetches per-project YAML alongside the spec. The `ProjectTag` field on the `.ndf.json` marker is removed in lockstep.
- **`ndf update` preflight short-circuit removed.** The `.ndf-pending-migration` existence check that previously refused re-delivery when a prior gate-fired run was incomplete is gone. Re-firing the gate is now idempotent — safer than the old short-circuit, whose recovery message ("run `/ndf-migrate` instead") was misleading when companion files needed re-fetching.

### Compatibility

- **No breaking changes for clean-shape clients.** v4.0.x clients without canary-shape v3 state continue to work unchanged.
- Framework v4.0.2 bumps `min_cli_version` to `2.3.1` so clients running the v3→v4 migration don't accidentally use CLI v2.2.0's removed `clearStalePendingMigrationFiles` path mid-walk.

### Paired framework release

Framework v4.0.2 ships alongside this CLI release with the self-authoring migration spec described above. Update the CLI first (`ndf self-update`), then run `ndf update` to pick up the framework patch.

---

## v2.3.0 — 2026-05-20

**`ndf self-update` subcommand.** Distinguishes updating the `ndf` CLI binary from `ndf update` (which updates the framework files in a project).

### What's new

- **`ndf self-update`** — new subcommand that prints channel-aware instructions for updating the CLI itself. Detects the install channel from the binary's path (Homebrew, Scoop, install.sh, install.ps1) and surfaces the matching update command. Falls back to listing all channels if detection is ambiguous (manual download, build from source). Does not replace the binary in place.
- **`ndf update --help` cross-references `ndf self-update`.** The two verbs now point at each other so the right one is always one step away.
- **`min_cli_version` halt message** now points at `ndf self-update` for the update path on v2.3.0+ binaries (the message text is baked into each binary, so users still on v2.2.0 or earlier will continue to see the previous "re-run the install one-liner from the onboarding email" wording until they update once).
- **`install.sh` and `install.ps1` post-install output** prints a one-line "To update ndf later, run: ndf self-update" pointer so first-time installers discover the verb before they need it.

### Compatibility

- No file-format or marker-schema change. No new flags on existing subcommands.
- `min_cli_version` unchanged.
- Pure additive: existing scripts and CI workflows are unaffected.

### Paired framework release

Framework v4.0.1 ships alongside this CLI release with a `METHODOLOGY.md` fix that now points at `ndf self-update` (with an explicit older-CLI fallback for clients still on v2.2.0). If you're on framework v4.0.0, running `ndf update` after updating the CLI will pick up the patch with a small expected diff in `METHODOLOGY.md`.

---

## v2.2.0 — 2026-05-20

> **Backfill note (2026-05-20):** v2.2.0 was tagged and shipped in mid-May 2026 alongside framework v4.0.0; the CHANGELOG entry below is documented here for completeness. If you already have CLI v2.2.0 installed, these capabilities have been live since you first updated to framework v4.0.0 — nothing changes on your end with this backfill.

**Paired with framework v4.0.0.** Five new capabilities required by the v3→v4 migration spec, designed to be reusable by future migrations.

### What's new

- **Uncommitted-state pre-flight on the gate-fired path.** `ndf update` now halts before writing any framework file when a migration is about to be delivered and the working tree is dirty. The check moved here from `/ndf-migrate.md` so framework files never land on a dirty tree in the first place. Halts on `git status` errors and cwd-lookup errors too (silent fail-open would risk contaminating a possibly-dirty tree). Detects `.ndf-pending-migration` already on disk and routes the user to `/ndf-migrate` rather than the generic "commit or stash" message.
- **Companion-file delivery for migration specs.** Migration-bearing updates can now pre-deliver optional per-project YAML companions alongside the spec. When the project's `.ndf.json` has a `project_tag` set, `ndf update` fetches `migrations/<name>.<project_tag>.map.yml` and `migrations/<name>.<project_tag>.yml` from the framework repo at gate-fired-update time and lands them at `.ndf-pending-migration-files/<basename>` for `/ndf-migrate` to read. Companions are optional — 404s are silently skipped.
- **Migration team-handoff marker.** Migration-bearing `ndf update` writes `.ndf-pending-handoff` at the end of the gate-fired run; the next `ndf update` (post-migration) reads it, prints a migration-specific team-handoff message exactly once, and removes the file. Concatenated across multiple pending migrations so clients catching up through several version jumps get one combined message.
- **`project_tag` field on `.ndf.json`.** Per-project marker field (committed) that the companion-file fetcher keys on. Optional (`omitempty`); unset by default. Per-developer config remains for tokens and global preferences. `ndf config show` now prints `project_tag` when set, symmetric with `fieldnotes_repo`.
- **`min_cli_version` enforcement edge.** Older CLIs running against the v4.0 framework manifest produce a clear "CLI too old" upgrade message (the v4.0 manifest's `min_cli_version` is `2.2.0`).

### Compatibility

- **Old CLIs reading new files:** unknown fields are ignored (`omitempty` posture).
- **New CLIs reading old files:** absent fields read as the zero value; companion fetch skipped when `project_tag` is empty; `consumePendingHandoff` on a tree with no marker is a no-op.
- v2.2.0 against v3.x manifests degrades gracefully — no migrations in the array, none of the new paths fire.

### When you need `project_tag`

Most projects do not need `project_tag` — it is optional and only consulted by the companion-file fetcher. Set it by editing `.ndf.json` directly when your project requires migration-specific input files distributed alongside a structural migration. Your Nandu contact will tell you the right slug if one applies.

---

## v2.1.3 — 2026-05-17

**Distribution-only release.** Source identical to v2.1.2. First release where both macOS and Windows binaries are signed.

### Distribution

- **macOS binaries are now Developer ID-signed and Apple-notarized.** `ndf-darwin-arm64` and `ndf-darwin-amd64` carry a Developer ID Application signature from `Nandu.ai GmbH` (Team ID `M43MMPX4K7`), use the hardened runtime, and have been submitted to Apple's notary service. Gatekeeper no longer shows the "cannot verify developer" prompt on first run — `ndf` installs and runs without any prompt. Existing macOS users get the signed binaries automatically on `brew upgrade nandu-org/tap/ndf` or the next `install.sh` re-run.
- **Windows binary remains Authenticode-signed** (unchanged from v2.1.2 — same publisher, Azure Artifact Signing Public Trust).
- **No source change.** v2.1.2 and v2.1.3 differ only in the signature bytes on the four signed binaries (both darwin binaries and the windows binary). The linux binary is unchanged.

### What this closes

- The "First-run prompts (current releases are unsigned)" warning block in README.md is now obsolete and has been removed.
- Both signing applications (Apple Developer Program, Azure Artifact Signing) from the v2.0.0 ship plan are complete.
- winget submission is now unblocked.

---

## v2.1.2 — 2026-05-10

**Distribution-only release.** Source identical to v2.1.1. First release with a signed Windows binary.

### Distribution

- **Windows binary is now Authenticode-signed** via Azure Artifact Signing (Public Trust certificate profile, publisher `Nandu.ai GmbH`). SmartScreen no longer shows "Windows protected your PC" on first run — `ndf.exe` installs and runs without any prompt. Existing Windows users get the signed binary automatically on `scoop update ndf` or the next `install.ps1` re-run.
- **macOS binaries remain unsigned** in this release (Apple Developer Program enrollment is pending). The one-time Gatekeeper prompt described in the README still applies on first run; this will go away in a later release once the Apple cert lands.
- **No source change.** v2.1.1 and v2.1.2 produce byte-identical binaries on macOS and Linux. Only `ndf-windows-amd64.exe` differs between the two releases (signature bytes appended).

---

## v2.1.1 — 2026-05-10

**Bug fix.** The team handoff message and the commit-and-push prompt no longer warn about a structural migration when no migration was triggered on this run.

### Fixed

- **Sentinel-aware handoff and commit prompt.** Previously, every `ndf update` against a project past v3.2.0 printed *"⚠️ THIS UPDATE INCLUDES A STRUCTURAL MIGRATION. After merging main, run /ndf-migrate in Claude Code"* — even when the migration gate was skipped because all migrations in the manifest were already applied. The CLI was reading the *total* count of migrations in the manifest, not the *pending* count for this run. Manifest entries stay forever (so old migrations stay listed indefinitely for clients catching up across version jumps), so the warning fired on every update for every client whose project had already been migrated. v2.1.1 reads the per-run gate-fired signal correctly: zero pending migrations → no warning, no extra "run /ndf-migrate" line in the handoff.
- The same fix applies to the inline `[Y/n] Commit and push these changes now?` prompt — it no longer treats "manifest has migration entries" as "we triggered a migration this run".

### Compatibility

- No behavior change for cases where a migration genuinely fires (the warning still appears on the run that delivers a fresh migration).
- No file-format or marker-schema change. v1.x and v2.0.x clients can run side-by-side; the bug-fix is purely in CLI output formatting on the post-update path.

### Companion fix

This pairs with framework v3.5.1 (released the same day): `/ndf-migrate` now exits cleanly when invoked with no pending migration. The two together close a confusion loop where users saw the misleading handoff, ran `/ndf-migrate`, and got an unfriendly halt. Either fix alone makes the situation safer; both together close the loop end-to-end.

---

## v2.1.0 — 2026-05-10

**Closes a UX gap around the per-project `fieldnotes_repo`.** No framework version bump, no manifest change, no breaking change.

### What's new

- **`ndf init` prompts for `--fieldnotes-repo` when omitted.** TTY-only — empty input keeps the existing warn-and-continue path, and CI / piped stdin behaves exactly as before (no hang, warning still emitted). The prompt mirrors how `ndf login` already prompts for missing tokens.
- **New subcommand `ndf config set fieldnotes-repo OWNER/REPO`.** Sets or updates the field-notes repo on an already-initialized project, replacing the previous "hand-edit `.ndf.json`" workaround. Must be run inside an ndf project (refuses with a clear hint otherwise).
- **Repo-slug validation.** All three input paths (`--fieldnotes-repo` flag, init prompt, `ndf config set` argument) now validate the value matches `OWNER/REPO` shape before persisting. Malformed input is rejected with an actionable error message.

### Why

Tokens get prompt-when-missing treatment in `ndf login`; the per-project `fieldnotes_repo` deliberately did not. Both are required for `/field-note` to work, so the asymmetry caused fresh installs to land in a half-configured state — tokens set, repo missing, no obvious recovery path. Setting the repo on an already-initialized project required hand-editing `.ndf.json`. v2.1.0 closes both gaps.

### Compatibility

- Existing scripts that pass `--fieldnotes-repo=<owner/repo>` are unaffected — flag values short-circuit the prompt.
- Existing CI pipelines that don't pass the flag continue to see the warning at end of init; behavior preserved exactly.
- Existing `.ndf.json` files are read and written with the same schema. No marker schema change.

---

## v2.0.1 — 2026-05-09

**Bug fix.** Fixes a config-path regression in v2.0.0 that prevented the Go binary from reading existing v1.x config files on macOS.

### Fixed

- **macOS config path.** v2.0.0 used Go's `os.UserConfigDir()`, which returns `~/Library/Application Support` on macOS — not `~/.config` where the bash CLI v1.x wrote `config.json`. The Go binary therefore couldn't see existing PATs and reported "no framework PAT configured" on every command. v2.0.1 hardcodes `~/.config/nandu/` on macOS + Linux (matching bash v1.x behavior), keeping `XDG_CONFIG_HOME` overrides intact and `%APPDATA%\nandu\` on Windows. Existing v1.x users on macOS no longer need to re-run `ndf login`.

### No other changes

No behavior changes beyond the path fix. Same binaries shipped via Homebrew, Scoop, `install.sh`, `install.ps1`, and direct download.

---

## v2.0.0 — 2026-05-09

**Full rewrite from bash to Go. Native Windows support. Single static binary on every platform.**

### What's new

- **Native Windows support.** `ndf.exe` runs from PowerShell, cmd, or any Windows shell. No WSL, no Git Bash, no install of additional tooling required.
- **Single static binary.** No bash, no `jq`, no `awk`, no `sed`, no `diff`, no `sha256sum`. Everything `ndf` needs is in the binary itself.
- **Package manager support.** Install via Homebrew (`brew install nandu-org/tap/ndf`) on macOS or Scoop (`scoop install ndf`) on Windows. Direct-download installers (`install.sh`, `install.ps1`) remain the fast-path option.
- **Faster.** Startup time on macOS dropped from ~250ms (bash + jq + awk forks) to ~10ms.

### What stays the same

- **Behavior.** Every subcommand, flag, prompt, and file format is byte-compatible with v1.3.x. Existing `.ndf.json` markers and `~/.config/nandu/config.json` files keep working untouched. Migration sentinels, the team handoff message format, the conflict-prompt UX — all preserved exactly.
- **Manifest protocol.** The framework repo's `manifest.json` schema is unchanged; v1.3.x and v2.0.x both consume it the same way.
- **Tokens.** Same env vars (`NDF_GITHUB_TOKEN`, `NDF_FIELDNOTES_TOKEN`), same config file location.

### What changes for you

- **macOS / Linux (Homebrew or curl install):** `brew upgrade ndf` (or re-run the curl one-liner) replaces your existing `~/.local/bin/ndf` bash script with the binary. First post-upgrade `ndf update` may show a one-time Gatekeeper prompt ("cannot verify developer") because v2.0.0 is unsigned. v2.0.1 will be Apple Developer ID-signed and notarized; the prompt disappears.
- **Windows:** install via Scoop or PowerShell one-liner. First run shows a one-time SmartScreen "Windows protected your PC" prompt because v2.0.0 is unsigned. Click **More info → Run anyway**. v2.0.1 will be Authenticode-signed by nandu.ai GmbH; the prompt disappears.

### Breaking changes

None for end-users. A few items relevant to maintainers:

- The bash `ndf.sh` script is no longer the source of truth. It remains in the repo as `ndf.sh.deprecated` for archival; do not edit it. New work goes into the Go source files.
- `min_cli_version` semantics unchanged, but the manifest field now compares against the Go binary's `CLIVersion`.

### Internal

- Stdlib-only Go implementation, plus `golang.org/x/term` for hidden password input on `ndf login`.
- GitHub Actions matrix release workflow (`.github/workflows/release.yml`) cross-compiles for darwin/arm64, darwin/amd64, linux/amd64, windows/amd64 on tag push and uploads binaries + checksums.txt to GitHub Releases.

---

## v1.3.1 — 2026-05-08 (final bash release)

The last bash CLI release. See `ndf.sh.deprecated` for archived source.

- Sentinel-aware migration gate. Before v1.3.1, `ndf update` would re-deliver migration specs every run when `manifest.migrations` was non-empty, even after `/ndf-migrate` had successfully applied them — producing an infinite loop where each `ndf update` halted at the migration gate. v1.3.1 added the `.ndf-migrations/<name>.complete` sentinel check: a migration is considered applied iff its sentinel file exists.

(See KB `Nandu Development Framework — Version History.md` for full historical entries; this CHANGELOG is the client-facing subset.)
