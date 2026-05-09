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

> **First-run prompts (current releases are unsigned):** until code signing ships, you'll see a one-time prompt the first time you run `ndf` on a new machine.
> - **Windows:** SmartScreen "Windows protected your PC" — click **More info → Run anyway**.
> - **macOS:** Gatekeeper "cannot verify developer" — open **System Settings → Privacy & Security → Open Anyway**, or `xattr -d com.apple.quarantine $(which ndf)` in a terminal.
>
> Both prompts clear permanently after acknowledgement. A future release will ship Authenticode-signed (Windows) and Apple-notarized (macOS) binaries from nandu.ai GmbH and the prompts will disappear entirely.

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

The `fieldnotes_repo` for the project lives in the project's `.ndf.json` (committed by the project owner), so cloning the project gives you the right value automatically.

**Scaffolding a NEW project from scratch:**

```bash
mkdir new-project && cd new-project
ndf init \
  --token=<framework_pat> \
  --fieldnotes-token=<fieldnotes_pat> \
  --fieldnotes-repo=<owner/repo>
```

`ndf init` writes the framework files and creates `.ndf.json` with the project's `fieldnotes_repo`. Commit `.ndf.json` so coworkers don't need to set the repo path themselves.

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

## Requirements

`ndf` itself has **no runtime dependencies** — single static binary. The only external tool it shells out to is `git`, and only when offering to commit + push framework changes after `ndf update`. If `git` isn't on `$PATH`, `ndf update` still completes; it just skips the optional auto-commit step.

## Where things live

- CLI source (this repo, public): [nandu-org/nandu-dev-framework-cli](https://github.com/nandu-org/nandu-dev-framework-cli)
- Framework files (private, requires PAT): [nandu-org/nandu-dev-framework](https://github.com/nandu-org/nandu-dev-framework)
- Per-developer config: `~/.config/nandu/config.json` on Unix / `%APPDATA%\nandu\config.json` on Windows (mode 0600 on Unix; created by `ndf login`)
- Per-project marker: `.ndf.json` in the project root (commit this — it's how `ndf update` knows what's installed)

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
