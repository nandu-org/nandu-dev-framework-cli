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

**First project on a machine:** provide your tokens with `--token` and `--api-token`. They are persisted to `~/.config/nandu/config.json` (mode 0600) for all subsequent `ndf` invocations and the `/field-note` slash command.

```bash
cd new-project
ndf init --token=ghp_xxxxx --api-token=ndf_xxxxx
```

**Subsequent projects on the same machine:** just `init`, no flags needed.

```bash
cd another-project
ndf init
```

**Update an existing ndf project:**

```bash
cd existing-project
ndf update
```

Pin to a specific version: `ndf update --version=3.0.0`. Clear the pin: `ndf update --latest`.

When a release includes a structural migration, `ndf update` pre-delivers the migration spec and stops with an instruction to run `/ndf-migrate` in Claude Code, then re-run `ndf update`. See METHODOLOGY.md (delivered into each project) for the full flow.

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

- `NDF_GITHUB_TOKEN` — overrides `github_pat` from the config file
- `NDF_API_TOKEN` — overrides `api_token` from the config file

When set, env vars take precedence over the config file. Useful in CI where you don't want to materialize `~/.config/nandu/config.json`.

## License

MIT for the CLI script in this repo. The framework files in `nandu-org/nandu-dev-framework` are under a separate commercial license — see that repo for details.
