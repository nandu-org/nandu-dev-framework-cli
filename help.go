package main

// Help text — kept as separate funcs so each subcommand can print its own
// detailed help via `ndf <cmd> --help`. Top-level `ndf help` prints the
// overview only.

func printHelpInit() {
	rawErr(`Usage: ndf init [flags]

Scaffold a NEW ndf project in the current directory.
Refuses on existing .ndf.json — use ` + "`ndf login`" + ` to set tokens for an existing project,
or ` + "`ndf update`" + ` to update an already-installed project.

Flags:
  --token=<framework_pat>              GitHub PAT, read-only on the framework repo
  --fieldnotes-token=<fieldnotes_pat>  GitHub PAT, write-only on the client's field-notes repo
  --fieldnotes-repo=<owner/repo>       The client's field-notes repo (written to .ndf.json).
                                       If omitted, ndf init prompts for it interactively
                                       (TTY only); in CI, the warning is emitted and you
                                       can set it later via ` + "`ndf config set fieldnotes-repo`" + `.
  --version=<x.y.z>                    Pin to a specific framework version (default: latest tag)

Tokens (--token, --fieldnotes-token) are persisted to the per-developer
config (~/.config/nandu/config.json on Unix, %APPDATA%\nandu\config.json on
Windows). The fieldnotes_repo is persisted to the project's .ndf.json
(per-project, committed) so coworkers cloning the project pick it up automatically.

Env vars NDF_GITHUB_TOKEN and NDF_FIELDNOTES_TOKEN override the config file
when set. fieldnotes_repo has no env-var override.

To set credentials WITHOUT scaffolding a project, use ` + "`ndf login`" + ` instead.`)
}

func printHelpLogin() {
	rawErr(`Usage: ndf login [flags]

Set per-developer credentials. Interactive by default — prompts for tokens
with hidden input (the values don't appear on screen or in shell history).

Flags (for non-interactive / CI use):
  --token=<framework_pat>              Framework PAT (read-only on the framework repo)
  --fieldnotes-token=<fieldnotes_pat>  Field-notes PAT (write-only on the client's field-notes repo)

If a flag is provided, that value is used directly (no prompt). If a flag is
omitted, the prompt offers the existing value (press Enter to keep it).

Tokens are saved to the per-developer config with mode 0600 on Unix.
Use ` + "`ndf config show`" + ` to verify the resolved state without exposing the raw values.`)
}

func printHelpConfig() {
	rawErr(`Usage: ndf config <subcommand> [args]

Subcommands:
  show                          Print the resolved per-developer + per-project config (PATs masked)
  set <key> <value>             Set a configuration key (currently: fieldnotes-repo OWNER/REPO)

Tokens are set via ` + "`ndf login`" + `, not ` + "`ndf config set`" + `.`)
}

func printHelpConfigSet() {
	rawErr(`Usage: ndf config set <key> <value>

Set a configuration key.

Supported keys:
  fieldnotes-repo OWNER/REPO    The project's field-notes repo. Persisted to .ndf.json
                                (per-project, committed). Must be run inside an ndf project.

Tokens (framework PAT, fieldnotes PAT) are set via ` + "`ndf login`" + `, not here.`)
}

func printHelpUpdate() {
	rawErr(`Usage: ndf update [flags]

Update the framework files in the current ndf project.

Note: ` + "`ndf update`" + ` updates the framework FILES in your project. To update the
` + "`ndf`" + ` CLI binary itself, run ` + "`ndf self-update`" + `.

Flags:
  --version=<x.y.z>   Set the project's pinned_version to X and update to it.
  --latest            Clear the pinned_version and update to the latest tag.
                      (Mutually exclusive with --version.)

With no flags, updates to the project's pinned_version (or latest if no pin).`)
}

func printHelpSelfUpdate() {
	rawErr(`Usage: ndf self-update

Print channel-aware instructions for updating the ` + "`ndf`" + ` CLI binary itself.

Detects the install channel from the binary's path (Homebrew, Scoop, install.sh,
install.ps1) and prints the matching update command. Falls back to listing all
channels if detection is ambiguous (manual download, build from source).

Does NOT replace the binary in place: package-manager state would diverge from
on-disk state and break the next ` + "`brew upgrade`" + ` / ` + "`scoop update`" + `.

To update the framework FILES in your project (distinct from updating the CLI),
use ` + "`ndf update`" + `.`)
}

func printHelp() {
	rawErr(`ndf — Nandu Development Framework CLI (v` + CLIVersion + `)

Usage: ndf <command> [flags]

Commands:
  init           Scaffold a NEW ndf project in the current directory
  login          Set per-developer credentials (interactive by default)
  update         Update the framework files in an existing ndf project
  self-update    Print instructions for updating the ndf CLI binary itself
  config show    Print the resolved config (per-developer + per-project), PATs masked
  config set     Set a config key (currently: fieldnotes-repo OWNER/REPO)
  version        Print the CLI version
  help           Print this help

Run ` + "`ndf <command> --help`" + ` for command-specific help.

Typical onboarding flow for joining an existing NDF project:
  1) Install the CLI via the install one-liner (or your platform's package manager)
  2) ndf login                          (set your tokens — interactive, hidden input)
  3) cd <project> && ndf update         (verify everything works)`)
}
