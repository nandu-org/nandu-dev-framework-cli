# ndf — Nandu Development Framework CLI

CLI for installing and updating the [Nandu Development Framework](https://github.com/nandu-org/nandu-dev-framework) (NDF) in your Claude Code projects.

NDF is the framework files; this is the CLI that fetches, installs, and updates them. The framework files themselves live in a private repo — this CLI uses a GitHub PAT (issued per licensed organization) to fetch them at install and update time.

`ndf` ships as a single static binary. No bash, no Python, no Node — nothing but the executable. Native support for macOS (Intel + Apple Silicon), Linux (x86_64), and Windows (x86_64).

## Install

### macOS — Homebrew (recommended)

```bash
brew install nandu-org/tap/ndf
```

### Windows — Scoop (recommended)

```powershell
scoop bucket add nandu https://github.com/nandu-org/scoop-bucket
scoop install ndf
```

### Windows — PowerShell one-liner

```powershell
iwr -useb https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.ps1 | iex
```

> **Signed releases (v2.1.3+):** `ndf` binaries are now Authenticode-signed on Windows (Azure Artifact Signing, Public Trust certificate, publisher `Nandu.ai GmbH`) and Developer ID-signed + Apple-notarized on macOS. No first-run prompts on either platform. Older releases (v2.0.x, v2.1.0–v2.1.1, and v2.1.2's macOS binaries) are unsigned and trip a one-time SmartScreen/Gatekeeper prompt — upgrade to v2.1.3+ to clear it.

### macOS / Linux — curl one-liner

```bash
curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.sh | bash
```

Installs `ndf` to `~/.local/bin/ndf` and adds that to your `$PATH` if it isn't already there.

### Manual download

Pre-built binaries for every release: <https://github.com/nandu-org/nandu-dev-framework-cli/releases>

```
ndf-darwin-arm64       — macOS Apple Silicon
ndf-darwin-amd64       — macOS Intel
ndf-linux-amd64        — Linux x86_64
ndf-windows-amd64.exe  — Windows x86_64
checksums.txt          — sha256 per file
```

Download, verify checksum, place on `$PATH`, `chmod +x` (Unix).

After install, verify:

```
ndf version
```

### Verifying the signature (optional)

Releases from v2.1.3 onwards are signed by `Nandu.ai GmbH`. If you want to verify the signature on the binary you installed:

**macOS:**
```bash
codesign -dv --verbose=4 $(which ndf) 2>&1 | grep -E "Authority|TeamIdentifier"
spctl --assess --type execute --verbose $(which ndf)
```
The `codesign` output should list `Developer ID Application: Nandu.ai GmbH` in its authority chain, and `spctl` should print `accepted, source=Notarized Developer ID`.

**Windows (PowerShell):**
```powershell
Get-AuthenticodeSignature (Get-Command ndf).Source | Format-List
```
`Status` should be `Valid` and the signer should be `Nandu.ai GmbH`.

**Linux:** binaries are not signed (no equivalent ecosystem). Verify integrity via `checksums.txt` published with each release.

## Use

**Joining an existing NDF project (most common case):**

```bash
ndf login              # interactive — prompts for both PATs (hidden input)
cd <project>
ndf update             # verifies your local copy is current
```

`ndf login` saves your tokens to the per-developer config:
- macOS / Linux: `~/.config/nandu/config.json` (mode 0600)
- Windows: `%APPDATA%\nandu\config.json`

The `fieldnotes_repo` for the project lives in the project's `.ndf/cli/install.json` (committed by the project owner), so cloning the project gives you the right value automatically.

**Scaffolding a NEW project from scratch:**

```bash
mkdir new-project && cd new-project
ndf init \
  --token=<framework_pat> \
  --fieldnotes-token=<fieldnotes_pat> \
  --fieldnotes-repo=<owner/repo>
```

`ndf init` writes the framework files and creates `.ndf/cli/install.json` with the project's `fieldnotes_repo`. Commit `.ndf/cli/install.json` so coworkers don't need to set the repo path themselves.

**Update an existing NDF project:**

```bash
cd existing-project
ndf update                       # latest tag (or pinned_version if set)
ndf update --version=3.1.0       # pin
ndf update --latest              # clear pin, take latest
```

**Verify your config:**

```bash
ndf config show                  # prints resolved config with PATs masked
```

When a release includes a structural migration, `ndf update` pre-delivers the migration spec and stops with an instruction to run `/ndf-migrate` in Claude Code, then re-run `ndf update`. See METHODOLOGY.md (delivered into each project) for the full flow.

After a non-no-op update, `ndf update` prints a **team handoff message** — a paste-ready block summarizing the version bump, what changed, and what coworkers need to do (`git pull`, `git merge main`, `/compact`). Designed for the updater to drop into team chat. See METHODOLOGY.md's "Framework updates during active development" section for the multi-developer workflow.

### Reading the project marker from external tools

Slash commands, hooks, and third-party tools should read the project marker through the CLI rather than via direct `jq` / `cat`:

```bash
ndf is-project                          # exit 0 = yes, 1 = no, 2 = error
ndf marker-path                          # print absolute path to the marker
ndf config get version                   # framework version
ndf config get pinned_version            # pinned version (empty when null)
ndf config get fieldnotes-repo --source  # repo + source ("marker" or "legacy-config") to stderr
```

Keys accept both kebab-case (`fieldnotes-repo`) and snake_case (`fieldnotes_repo`). Closed key set — tokens are deliberately NOT exposed (use `ndf config show` for the masked view).

Exit codes across these subcommands: 0 = success, 1 = absent (`is-project` only), 2 = internal error (stderr message plus `ndf:internal-error` stdout marker for environments that swallow stderr).

This indirection lets future relocations of the marker file (or schema reshape) become CLI-internal refactors rather than breaking changes for every consumer.

## Update the CLI

`ndf update` updates the **framework files in a project**; it does NOT update the `ndf` CLI binary itself. To update the CLI, run:

```bash
ndf self-update
```

`ndf self-update` detects how this binary was installed (Homebrew, Scoop, install.sh, install.ps1) and prints the matching update command. It does **not** replace the binary in place — running `brew` / `scoop` / the install one-liner keeps your package manager's recorded state in sync with what's on disk.

The commands `ndf self-update` will surface, depending on your install channel:

| Install channel | Update command |
|---|---|
| Homebrew (macOS) | `brew upgrade nandu-org/tap/ndf` |
| Scoop (Windows) | `scoop update ndf` |
| `install.sh` (macOS / Linux) | re-run `curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.sh \| bash` (idempotent) |
| `install.ps1` (Windows) | re-run `iwr -useb https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.ps1 \| iex` (idempotent) |
| Manual download | grab the new binary from <https://github.com/nandu-org/nandu-dev-framework-cli/releases> |

After updating, verify with `ndf version`.

> **`ndf update` vs `ndf self-update`:** `update` for the framework files in your project; `self-update` for the CLI itself. The two verbs point at each other from their help text.

## Requirements

`ndf` itself has **no runtime dependencies** — single static binary. The only external tool it shells out to is `git`, and only when offering to commit + push framework changes after `ndf update`. If `git` isn't on `$PATH`, `ndf update` still completes; it just skips the optional auto-commit step.

## Where things live

- CLI source (this repo, public): [nandu-org/nandu-dev-framework-cli](https://github.com/nandu-org/nandu-dev-framework-cli)
- Framework files (private, requires PAT): [nandu-org/nandu-dev-framework](https://github.com/nandu-org/nandu-dev-framework)
- Per-developer config: `~/.config/nandu/config.json` on Unix / `%APPDATA%\nandu\config.json` on Windows (mode 0600 on Unix; created by `ndf login`)
- Per-project marker: `.ndf/cli/install.json` (commit this — it's how `ndf update` knows what's installed)

## Env vars (CI use)

- `NDF_GITHUB_TOKEN` — overrides `framework_pat` from the config file
- `NDF_FIELDNOTES_TOKEN` — overrides `fieldnotes_pat` from the config file

When set, env vars take precedence over the config file. Useful in CI where you don't want to materialize the config file. `fieldnotes_repo` has no env-var override (it's not a secret); CI workflows that need it should write the marker directly.

## Building from source

```bash
git clone https://github.com/nandu-org/nandu-dev-framework-cli
cd nandu-dev-framework-cli
go build -o ndf .
./ndf version
```

Requires Go 1.22+. No external Go dependencies beyond `golang.org/x/term` (vendored on `go mod tidy`).

## License

MIT for the CLI in this repo. The framework files in `nandu-org/nandu-dev-framework` are under a separate commercial license — see that repo for details.
