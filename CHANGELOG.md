# Changelog

## v2.8.2 — 2026-07-16

**A brand-new project can now complete its first `ndf update`.** Until this release it could not: `ndf init` scaffolded the project but recorded nothing about which structural migrations applied to it, so the first `ndf update` reported every migration in the framework as pending on a project that had never had anything to migrate — and `/ndf-migrate` then stopped on the first one, because it looks for planning files a new project has not created yet. Nothing wrote the completion marker, so the same thing happened on every later `ndf update`. If you have an existing project, this never affected you; your migration history is already on disk.

### Fixed

- **`ndf init` now records the framework's migrations as already satisfied.** A project created by `ndf init` is in the installed version's shape from the start, so every migration up to that version is complete by definition — there was never anything for them to do. `ndf init` now writes that into `.ndf/cli/sentinels/` instead of leaving it empty. The migration gate still fires normally for any migration a project genuinely needs: if you are an existing project catching up, nothing about your update changes.
- **`ndf init --help` printed a corrupted Windows config path.** It rendered `%!A(MISSING)PPDATA%!\(MISSING)nandu\config.json` instead of `%APPDATA%\nandu\config.json`. Text only — the CLI always read and wrote the correct location.

### Removed

- **`ndf init --version=<x.y.z>` is gone. `ndf init` always installs the latest framework version.** To start a project on an older version, init first and then run `ndf update --version=<x.y.z>`, which pins the project to that version and moves it there in one step — the same thing the flag did, via the command that already documents it. `ndf update --version=` and `ndf update --latest` are unchanged. A script still passing `--version=` to `ndf init` will now stop with `unknown init flag`.

### Compatibility

- **No manifest schema or format change. No `min_cli_version` bump.** Framework files are untouched — this is a CLI-only release. Existing projects are unaffected: the fix changes only what `ndf init` records when it creates a project. Patch bump 2.8.1 → 2.8.2 — nothing you can do today becomes impossible.

---

## v2.8.1 — 2026-07-16

**Coworkers get the right instructions when framework v4.16.0's migration lands.** `ndf update` now stages a migration-specific team-handoff message for `v4.15-to-v4.16-settings-split`, the way it already does for the v3→v4, v4.0→v4.2 and v4.3→v4.4 migrations.

### Fixed

- **The team handoff for framework v4.16.0 said `/compact`. It needs to say `/clear`.** v4.16.0 moves the hooks out of `.claude/settings.json` into their own files, and Claude Code reads hooks at session start — `/compact` does not re-read them. Without a handoff entry for that migration, coworkers got the standard message ("drift fixes", `/compact`) and kept running the pre-split hooks until they happened to restart their session. The new message says `/clear`, explains that `settings.json` is now theirs, warns that the gates now fire in git worktrees where they previously did not, and tells anyone who had extended the plan-check's source directories to set `NDF_SOURCE_ROOTS` in `.claude/hooks/hooks.config.sh`.
- **Documented when `min_cli_version` has to move.** The comments on the `user_customizable` manifest field said adding the field never forces a `min_cli_version` bump. That is true of *adding* it and does not generalise: the reasoning holds only while a flagged file's content is unchanged. Corrected in place, with the general rule — a version floor tracks any guarantee the manifest makes that only the CLI can keep, not just the manifest's format. The v2.6.0 compatibility note is annotated to match. No behaviour change.

### Compatibility

- **No manifest schema or format change. No `min_cli_version` bump.** Framework files are untouched — this is a CLI-only release. Patch bump 2.8.0 → 2.8.1: a handoff dispatcher entry is data for an existing capability, not a new one (same shape as v2.3.2).

---

## v2.8.0 — 2026-07-14

**`ndf init` and `ndf update` now operate on the project in your current directory.** They resolve the project marker, framework files, and git actions all from the directory you run them in, and no longer follow `$CLAUDE_PROJECT_DIR`. This fixes a latent bug where, if `$CLAUDE_PROJECT_DIR` pointed at a project root different from your current directory (e.g. running from a subfolder under an editor integration), `ndf update` could record the update against one directory while writing the framework files into another.

### What's changed

- **`ndf init` / `ndf update` are anchored to the current directory.** Run them from the project root. If you run them somewhere that isn't an ndf project, they refuse with "not an ndf project" instead of acting on a different directory. In the common case — a plain terminal at the project root — behavior is unchanged.
- **The read commands are unchanged.** `ndf is-project`, `ndf marker-path`, `ndf config get`, `ndf config show`, and `ndf version` still honor `$CLAUDE_PROJECT_DIR` (editor integrations and hooks rely on that to locate the project from any directory). `ndf config set` also still honors it — it only writes the marker, so it was never affected by the bug.
- **`ndf config show` now prints the marker's real resolved path** instead of a `./…`/"in cwd" label that could be misleading when `$CLAUDE_PROJECT_DIR` pointed elsewhere.

### Compatibility

- **No manifest schema or format change. No `min_cli_version` bump.** Framework files are untouched — this is a CLI-only release. The only behavior change is that `ndf init`/`ndf update` no longer follow `$CLAUDE_PROJECT_DIR`; run them from the project root (the normal case is unaffected).

---

## v2.7.0 — 2026-07-14

**`ndf version` now reports the installed framework version too.** Run inside an NDF project, `ndf version` prints the framework version on a second line — the number you pin and update — not just the CLI binary version.

### What's changed

- **`ndf version` prints a framework line when in a project.** In a directory with an NDF install (cwd or `$CLAUDE_PROJECT_DIR`), the output is now two lines:

  ```
  ndf v2.7.0
  framework v4.15.0
  ```

  A pinned project shows `framework v4.15.0 (pinned: v4.15.0)`. Outside an NDF project, only the CLI line prints, exactly as before.
- **The CLI-version line is unchanged.** The first line stays `ndf v<version>`, so anything that reads the CLI version from `ndf version` keeps working. For a scripted read of the framework version, use `ndf config get version` (the machine-readable value) rather than parsing `ndf version`.
- **A corrupt project marker no longer breaks `ndf version`.** If `install.json` can't be read, `ndf version` still prints the CLI version and exits 0, with a warning on stderr — it never fails outright.
- **`ndf version --help` now prints help** instead of ignoring the flag and printing the version, matching every other subcommand. `ndf --version` / `ndf -v` still print the version.

### Also fixed (same consistency sweep)

- **`ndf config show --help` now prints help** instead of running the command and ignoring the flag — bringing it in line with `ndf config set --help` and `ndf config get --help`.
- **`ndf config get` help now describes exit codes accurately.** A malformed or absent project marker makes `ndf config get <key>` print an empty value and exit **0** (not exit 2) — by design. To tell "the value is empty" apart from "there's no project, or the marker is corrupt", run `ndf is-project` (which exits 2 on a corrupt marker). The previous help text wrongly implied a malformed marker exits 2.

### Compatibility

- **No manifest schema or format change. No `min_cli_version` bump.** Framework files are untouched — this is a CLI-only release. Older CLIs simply keep printing the CLI line only. No behavior change to `ndf config get`/`ndf config show` beyond the new `--help` handling — only their documentation was corrected.

---

## v2.6.0 — 2026-06-19

**Defense-in-depth for client-customized framework files.** A new optional manifest field, `user_customizable: true`, marks files the framework scaffolds once but you own thereafter — currently the pre-commit test hook (`.claude/hooks/pre-commit-tests.sh`), the placeholder you replace with your project's real test command. `ndf update` now guarantees it will never silently overwrite such a file.

### What's changed

- **`ndf update` never silent-replaces a `user_customizable` file.** The decision compares the file on disk directly against the framework's version, independent of the project marker's recorded checksums: absent on disk → the placeholder is created; identical to the framework → left alone; **different from the framework → your version is preserved, never overwritten.** Any preserved file is listed in a short post-update summary.
- Because the decision ignores the marker, the guarantee holds even if the marker's `installed_checksums` entry for the file is missing or stale (for example, after a multi-version update that relocated the marker).

### Compatibility

- **The new `user_customizable` manifest field is optional. No `min_cli_version` bump.** Older CLIs ignore the field (JSON tolerates unknown keys) and continue to skip the unchanged placeholder via the existing "framework hasn't changed it" path. Surfaces with framework v4.7.3, which flags `pre-commit-tests.sh`.

  > **Note added 2026-07-16 (framework v4.16.0).** The note above was correct for v4.7.3 and does not generalise — "the **unchanged** placeholder" is the condition it rests on. It holds only while a flagged file's content stays the same, because `installed_sha == manifest_sha` short-circuits before the update logic is reached. Framework v4.16.0 changes the content of both flagged files, so that short-circuit no longer applies and a CLI older than v2.6.0 — which ignores the flag — would treat them as ordinary tracked files. **Framework v4.16.0 therefore requires CLI v2.6.0 or newer.** If you are on an older CLI, `ndf update` will tell you; run `ndf self-update`.

---

## v2.5.2 — 2026-06-11

**Neutral placeholders in prompts and error messages.** The `ndf init` field-notes-repo prompt and the repo-slug validation error now illustrate the OWNER/REPO shape as `nandu-org/Example-FieldNotes`.

### What's changed

- **`ndf init`'s interactive field-notes-repo prompt and the `fieldnotes_repo` validation error** use a neutral placeholder example instead of a specific repo name. Test fixtures updated to match. No flag, schema, or behavior change.

### Compatibility

- **No manifest schema or format change. No `min_cli_version` bump on any framework.** Every code path other than the two example strings is byte-for-byte unchanged.

---

## v2.5.1 — 2026-06-05

**Bug fix: `ndf update` never silently overwrites a client's own file when creating a net-new framework file.** The per-file loop's net-new branch fetched the file unconditionally (atomic temp+rename, which overwrites without checking), so a client who had authored their own untracked file at the exact path a brand-new framework file targets would have it clobbered with no warning. The branch now checks the destination on disk first and falls to a diff-and-prompt on collision instead.

### What's fixed

- **Net-new framework files no longer clobber untracked files on disk.** When a manifest file is absent from the installed checksums (a net-new file) but a file already exists at its destination path, `ndf update` now surfaces a diff of your existing file vs the framework's new file and prompts `[r]eplace / [s]kip / [b]ackup-and-replace`, defaulting to skip — the same machinery the existing conflict path uses, but with wording accurate to this case (a pre-existing untracked file, not a file changed both locally and upstream). The normal case — destination path empty — keeps the previous silent-create behavior unchanged.

### Compatibility

- **No manifest schema or format change. No `min_cli_version` bump on any framework.** Pure bugfix on the `ndf update` file loop; every other code path is byte-for-byte unchanged.
- Surfaces with framework v4.7.0, the first release in a while to add a net-new agent file (`.claude/agents/acceptance-verifier.md`) — exactly the kind of brand-new path where a client might already have authored a same-named file.

---

## v2.5.0 — 2026-05-26

**CLI-managed state consolidated under `.ndf/cli/`.** Pairs with framework v4.4.0 (to ship after propagation). The CLI now reads the project marker at `.ndf/cli/install.json` and writes new sentinels and pending markers under `.ndf/cli/sentinels/`, `.ndf/cli/pending-migration`, and `.ndf/cli/pending-handoff`.

### What's new

- **New on-disk layout for CLI-managed state.** `ndf init` creates the marker at `.ndf/cli/install.json` (was `.ndf.json` at the project root); migration sentinels land in `.ndf/cli/sentinels/` (was `.ndf-migrations/`); transient markers land under `.ndf/cli/` (was `.ndf-pending-*` at the project root).
- **Team-handoff dispatcher case for `v4.3-to-v4.4-cli-state-relocation`.** `ndf update` now emits a paste-ready coworker-recovery message on the post-migration re-run that instructs teammates to run `ndf self-update` before pulling main, so older CLIs don't fall over after the migration relocates the marker.

### Compatibility

- **Backwards-compatible read of pre-relocation marker layout during the migration window.** Existing projects whose on-disk state has not yet been relocated by the framework v4.4.0 migration continue to work transparently — `ndf is-project`, `ndf marker-path`, `ndf config get`, `ndf update`, and `ndf config set` all behave correctly against both pre- and post-relocation layouts.
- **Pairs with framework v4.4.0**, which contains the migration spec that performs the on-disk relocation (`migrations/v4.3-to-v4.4-cli-state-relocation.md`). Framework v4.4.0 will bump `min_cli_version` to `2.5.0` to ensure clients have these read/write contracts in place before the manifest version that triggers the move can land.
- **Pure-additive for pre-migration clients.** No flag, schema, or behavior change on any other code path. CLI subcommands behave identically against pre-v4.4.0 projects; the only externally observable change is the location where a fresh `ndf init` writes its files.

### Verification

- New `scripts/verify-dual-path.sh` exercises five fixture scenarios covering the catch-up window between CLI v2.5.0 and framework v4.4.0 (marker at OLD only, sentinels at OLD only, pending-handoff at OLD only, fresh write at NEW, read-OLD-then-write-NEW). Added to `RELEASE.md` as a pre-flight check alongside `scripts/verify-show.sh`.

### Framework pairing (post-propagation)

Framework v4.4.0 ships after CLI v2.5.0 has propagated through the distribution channels (Homebrew, Scoop, install scripts). The framework release contains `migrations/v4.3-to-v4.4-cli-state-relocation.md` — a one-time `/ndf-migrate` step that moves existing on-disk state to the new layout. After framework v4.4.0 lands, run `ndf update` to trigger the migration delivery, then `/ndf-migrate` in Claude Code to apply it.

---

## v2.4.0 — 2026-05-26

**CLI-as-contract for `.ndf.json` reads.** Three new read-only subcommands mediate external access to the project marker so consumers no longer hit the file directly — future moves or reshapes of the marker become CLI-internal refactors rather than breaking changes.

### What's new

- **`ndf is-project`** — exit 0 if cwd (or `$CLAUDE_PROJECT_DIR`) contains a parseable `.ndf.json`, exit 1 if absent, exit 2 on internal error. Silent on 0 and 1 — caller decides what to print. Replaces the `test -f .ndf.json` idiom external tools were using.
- **`ndf marker-path`** — print the absolute resolved path to `.ndf.json` the CLI would consult (honors `$CLAUDE_PROJECT_DIR`). Does not check existence; pair with `ndf is-project` if you need that.
- **`ndf config get <key> [--source]`** — print a single config value to stdout. Closed key set: `version`, `pinned_version`, `fieldnotes_repo`. Both kebab-case (`fieldnotes-repo`) and snake_case (`fieldnotes_repo`) accepted. `--source` flag prints `marker` or `legacy-config` to stderr (useful for tracing where a value resolved). PATs deliberately NOT exposed via this command — use `ndf config show` for the masked view.
- **`markerPath()` honors `$CLAUDE_PROJECT_DIR`.** The resolver now returns an absolute path rooted at `$CLAUDE_PROJECT_DIR` (or cwd if unset), fixing a long-standing comment/code drift. All existing callers (`ndf init`, `ndf update`, `ndf config show`, `ndf config set fieldnotes-repo`) pick this up automatically.

### Exit code conventions for read-only mediated reads

Across `ndf is-project`, `ndf marker-path`, `ndf config get`: 0 = success, 1 = absent (only `is-project` uses this), 2 = internal error (stderr message plus an `ndf:internal-error` stdout marker so callers in environments that swallow stderr can still detect the failure).

### Compatibility

- **Pure-additive.** No flag, schema, behavior, or output change on any other code path.
- **`cmdConfigShow` rendering is byte-for-byte preserved** under existing inputs, with one prose-only update: the legacy-config annotation reads `(source: legacy-config — v1.2.x layout)` instead of the prior `(legacy v1.2.x location; v1.3.0+ reads per-project .ndf.json first)`. Golden-file check in `scripts/verify-show.sh` enforces no other rendering drift.
- **No manifest schema change.** No `min_cli_version` bump on any shipped framework. The framework-side migration to the new subcommands (`/field-note`, `/ndf-migrate`, allow-list) is a separate framework release (v4.3.0) that bumps `min_cli_version` to `2.4.0` once this CLI has propagated.
- Existing scripts that read `.ndf.json` directly continue to work — direct-file access is not removed, just no longer the contracted reading path.

---

## v2.3.2 — 2026-05-22

**Paired with framework v4.2.0.** Adds the team-handoff dispatcher case for the new `v4.0-to-v4.2-heavyweight-phases` migration so coworkers running `ndf update` after the v4.2 framework lands see a paste-ready chat message covering the artifact-tree change.

### What's new

- **Team-handoff text for `v4.0-to-v4.2-heavyweight-phases`.** The `migrationHandoffText` dispatcher gains a case for the new migration. The message covers: phased features now use per-phase folders for `spec.md` / `design.md` / `tasks.md`; coworkers with uncommitted local edits to a phased feature's feature-level files should compare against the new `phase-M-<phase-slug>/` subfolders and relocate edits; per-phase work happens on per-phase branches that `/implement` cuts when you pick a phase. Flat features are unchanged.

### Compatibility

- No flag, schema, or behavior change on any other code path. Purely additive: one new switch case + one new constant.
- Framework v4.2.0's `min_cli_version` stays at `2.3.1`. Clients still on CLI v2.3.1 can still install framework v4.2.0 and run the migration; they fall through to the default empty handoff (the standard "pull main + /compact" block from `printTeamHandoff`). Upgrading the CLI is recommended but not required.

### Paired framework release

Framework v4.2.0 ships alongside this CLI release with heavyweight per-phase artifacts for phased features. Update the CLI first (`ndf self-update`), then run `ndf update` to pick up the framework.

---

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
