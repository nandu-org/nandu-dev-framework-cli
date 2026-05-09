# Changelog

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
