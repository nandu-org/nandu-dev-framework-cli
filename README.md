# ndf — Nandu Development Framework CLI

Bash CLI for installing and updating the [Nandu Development Framework](https://github.com/nandu-org/nandu-dev-framework) (NDF) in your Claude Code projects.

NDF is the framework files; this is the CLI that fetches, installs, and updates them. The framework files themselves live in a private repo — this CLI uses a GitHub PAT (issued per licensed organization) to fetch them at install and update time.

## Install

One-liner — no auth required:

```bash
curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.sh | bash
```

The installer downloads `ndf` to `~/.local/bin/ndf` and (if needed) adds `~/.local/bin` to your `$PATH` by appending one line to your `~/.zshrc` or `~/.bashrc`. Idempotent — safe to re-run to upgrade in place.

After install, either open a new terminal or run `exec $SHELL -l`, then verify:

```bash
ndf version
```

### Manual install (if you'd rather not pipe to bash)

```bash
mkdir -p ~/.local/bin
curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/ndf.sh \
  -o ~/.local/bin/ndf && chmod +x ~/.local/bin/ndf
```

Then add `~/.local/bin` to your `$PATH` if it isn't already:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc on bash
exec $SHELL -l
```

## Use

**Joining an existing NDF project (most common case):**

```bash
ndf login              # interactive — prompts for both PATs (hidden input)
cd <project>
ndf update             # verifies your local copy is current
```

`ndf login` saves your tokens to `~/.config/nandu/config.json` (mode 0600). The `fieldnotes_repo` for the project lives in the project's `.ndf.json` (committed by the project owner), so cloning the project gives you the right value automatically.

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

Pin to a specific version: `ndf update --version=3.0.0`. Clear the pin: `ndf update --latest`.

When a release includes a structural migration, `ndf update` pre-delivers the migration spec and stops with an instruction to run `/ndf-migrate` in Claude Code, then re-run `ndf update`. See METHODOLOGY.md (delivered into each project) for the full flow.

After a non-no-op update, `ndf update` prints a **team handoff message** — a paste-ready block summarizing the version bump, what changed, and what coworkers need to do (`git pull`, `git merge main`, `/compact`). Designed for the updater to drop into team chat. See METHODOLOGY.md's "Framework updates during active development" section for the multi-developer workflow.

## Requirements

- Bash, curl, jq, diff, sed, awk
- sha256sum (Linux) or shasum (macOS)
- macOS, Linux, or Windows under WSL — Claude Code itself requires WSL on Windows, so the CLI does too. Native PowerShell/cmd is not supported.

## Where things live

- CLI source (this repo, public): [nandu-org/nandu-dev-framework-cli](https://github.com/nandu-org/nandu-dev-framework-cli)
- Framework files (private, requires PAT): [nandu-org/nandu-dev-framework](https://github.com/nandu-org/nandu-dev-framework)
- Per-developer config: `~/.config/nandu/config.json` (mode 0600; created by `ndf init`)
- Per-project marker: `.ndf.json` in the project root (commit this — it's how `ndf update` knows what's installed)

## Env vars (CI use)

- `NDF_GITHUB_TOKEN` — overrides `framework_pat` from the config file
- `NDF_FIELDNOTES_TOKEN` — overrides `fieldnotes_pat` from the config file

When set, env vars take precedence over the config file. Useful in CI where you don't want to materialize `~/.config/nandu/config.json`. `fieldnotes_repo` has no env-var override (it's not a secret); CI workflows that need it should write the config file directly.

## License

MIT for the CLI script in this repo. The framework files in `nandu-org/nandu-dev-framework` are under a separate commercial license — see that repo for details.
