package main

// Help text — kept as separate funcs so each subcommand can print its own
// detailed help via `ndf <cmd> --help`. Top-level `ndf help` prints the
// overview only.

func printHelpInit() {
	rawErr(`Usage: ndf init [flags]

Scaffold a NEW ndf project in the current directory.
Refuses on an existing ndf project — use ` + "`ndf login`" + ` to set tokens for an existing project,
or ` + "`ndf update`" + ` to update an already-installed project.

Flags:
  --token=<framework_pat>              GitHub PAT, read-only on the framework repo
  --fieldnotes-token=<fieldnotes_pat>  GitHub PAT, write-only on the client's field-notes repo
  --fieldnotes-repo=<owner/repo>       The client's field-notes repo (written to .ndf/cli/install.json).
                                       If omitted, ndf init prompts for it interactively
                                       (TTY only); in CI, the warning is emitted and you
                                       can set it later via ` + "`ndf config set fieldnotes-repo`" + `.

ndf init always installs the latest framework version. To start on an older one,
init first, then ` + "`ndf update --version=<x.y.z>`" + `.

Tokens (--token, --fieldnotes-token) are persisted to the per-developer
config (~/.config/nandu/config.json on Unix, %%APPDATA%%\nandu\config.json on
Windows). The fieldnotes_repo is persisted to the project's .ndf/cli/install.json
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
  get <key> [--source]          Print a single config value to stdout (version, pinned_version, fieldnotes_repo)

Tokens are set via ` + "`ndf login`" + `, not ` + "`ndf config set`" + `.`)
}

func printHelpConfigShow() {
	rawErr(`Usage: ndf config show

Print the resolved configuration: the per-developer config (tokens, masked) and,
when run inside an NDF project, the per-project marker (framework version,
pinned version, field-notes repo) plus the resolved field-notes repo for this
directory.

Read-only; prints nothing sensitive in the clear. Tokens are set via ` + "`ndf login`" + `.`)
}

func printHelpConfigGet() {
	rawErr(`Usage: ndf config get <key> [--source]

Print a single config value to stdout. Mediates external reads of the
project marker so consumers don't depend on its on-disk location or schema.

Keys (kebab or snake form both accepted):
  version            framework version from the marker
  pinned_version     pinned version from the marker (empty when null)
  fieldnotes_repo    project field-notes repo (marker, then legacy config)

Flags:
  --source           print the resolution source ("marker" or "legacy-config")
                     to stderr before exit. Useful for callers needing to
                     know whether a value came from the per-project marker
                     or the per-developer legacy config.

Exit codes:
  0  resolved (value printed; may be empty). A malformed or absent marker also
     exits 0 with an empty value — run ` + "`ndf is-project`" + ` (exit 2 on a corrupt
     marker) if you need to tell "no value" apart from "no/corrupt project".
  2  unknown key, or a bad flag to this command (stderr message + ndf:internal-error stdout marker)

Tokens are deliberately NOT exposed here — use ` + "`ndf config show`" + ` (masked).`)
}

func printHelpConfigSet() {
	rawErr(`Usage: ndf config set <key> <value>

Set a configuration key.

Supported keys:
  fieldnotes-repo OWNER/REPO    The project's field-notes repo. Persisted to .ndf/cli/install.json
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

With no flags, updates to the project's pinned_version (or latest if no pin).

When the release includes a structural migration, its spec is delivered to the
folder that release's ` + "`/ndf-migrate`" + ` reads (` + "`.ndf/cli/migrations/`" + ` for releases
that name it; the project-root ` + "`migrations/`" + ` for earlier ones) and the update
stops until ` + "`/ndf-migrate`" + ` has run. When every migration is already applied,
spec files of applied migrations that an earlier release left behind are
removed; the ones your repository tracks are listed as removed and included
in the commit offered below; a folder goes only when that left it empty. An
unfinished ` + "`/ndf-migrate`" + ` run stops the update until you have finished it.

Without an interactive terminal, every conflict and collision prompt
auto-answers "skip": nothing is replaced, the skipped files are listed at the
end of the run, and re-offer marks survive — an auto-skipped file gains one
if it had none (an automatic skip is not a decision; the marks are stored in
the committed install record) — re-run interactively to decide them.

After the file changes, if the project is a git repo with uncommitted framework
changes, ndf OFFERS to commit and push them (staging only its own changes) —
default is no, and without an interactive terminal it never commits or pushes:
it prints the commands for you to run instead. Files you decline at a version
delta stay at their previous version and re-surface on the next update; a file
you decline at a re-offer prompt stays yours and stops being offered
(` + "`ndf verify --fix`" + ` re-opens it). ` + "`ndf verify`" + ` shows the install state.`)
}

func printHelpVerify() {
	rawErr(`Usage: ndf verify [--fix]

Check every framework file recorded in the project's install record against
what is actually on disk. Offline — no network. Verdicts per file:

  ok            matches the last version the framework delivered (not listed)
  customized    differs or absent, but the file is yours by design
  unclassified  differs, but the record predates CLI v2.9.0 — run
                ` + "`ndf update`" + ` once to refresh it, then re-run verify
  AWAITING      marked for re-offer — by --fix, or by any file skipped
                without a typed decision (a declined collision, an
                unattended run's auto-skip, a failed replace); the next
                interactive ` + "`ndf update`" + ` prompts
  DRIFTED       differs and is framework-owned: either you edited it, or a
                past update never delivered it
  MISSING       no file on disk (the next ` + "`ndf update`" + ` restores it)

Exit codes: 0 = clean (unclassified counts as clean), 1 = drifted/missing/
awaiting, 2 = usage or environment error. A successful --fix exits 0, so
` + "`ndf verify --fix && ndf update`" + ` chains as documented.

  --fix        (interactive only) Mark the drifted files for re-offer: the
               next ` + "`ndf update`" + ` shows each one as a diff and you decide
               per file whether to take the framework's version or keep
               yours.`)
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

func printHelpVersion() {
	rawErr(`Usage: ndf version

Print the ndf CLI binary version. When run inside an NDF project (cwd or
$CLAUDE_PROJECT_DIR contains a project marker), also print the installed
framework version on a second line — and, when the install record shows
framework-owned files differing from what was delivered, a third line with
the count:

  ndf v` + CLIVersion + `
  framework vX.Y.Z            (plus " (pinned: vX.Y.Z)" when the project pins a version)
  N file(s) differ from the recorded install — run ` + "`ndf verify`" + `

On a record too old to classify (pre-v2.9.0), the third line says the check
cannot run and points at ` + "`ndf update`" + `.

The first line (the CLI version) is stable regardless of project context.
` + "`ndf --version`" + ` and ` + "`ndf -v`" + ` are aliases.

For a scripted read of the framework version, use ` + "`ndf config get version`" + `
(the machine-readable contract) rather than parsing this output. To see the
full resolved config, use ` + "`ndf config show`" + `.`)
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
  config get     Print a single config value (version, pinned_version, fieldnotes_repo)
  is-project     Exit 0 if cwd (or $CLAUDE_PROJECT_DIR) is an NDF project, 1 if not
  marker-path    Print the absolute path to the project marker the CLI would consult
  verify         Check installed framework files against the install record (offline)
  version        Print the ndf CLI version (plus the framework version when in a project)
  help           Print this help

Run ` + "`ndf <command> --help`" + ` for command-specific help.

Typical onboarding flow for joining an existing NDF project:
  1) Install the CLI via the install one-liner (or your platform's package manager)
  2) ndf login                          (set your tokens — interactive, hidden input)
  3) cd <project> && ndf update         (verify everything works)`)
}
