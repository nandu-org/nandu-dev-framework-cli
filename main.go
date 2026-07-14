// ndf — Nandu Development Framework CLI.
//
// Subcommands:
//
//	ndf init        [--token=<framework_pat>] [--fieldnotes-token=<fieldnotes_pat>]
//	                [--fieldnotes-repo=<owner/repo>] [--version=<x.y.z>]
//	    Scaffold a NEW NDF project. Refuses on an existing ndf project — use
//	    `ndf login` to set tokens for an existing project.
//
//	ndf login       [--token=<framework_pat>] [--fieldnotes-token=<fieldnotes_pat>]
//	    Set per-developer credentials. Interactive by default (hidden input);
//	    accepts flags for non-interactive use (CI).
//
//	ndf update      [--version=<x.y.z>] [--latest]
//	    Update an existing NDF project to the target framework version.
//
//	ndf self-update
//	    Print channel-aware instructions for updating the ndf CLI itself.
//	    Does not replace the binary — keeps package-manager state authoritative.
//
//	ndf config show
//	    Print the resolved config with PATs masked.
//
//	ndf config set fieldnotes-repo OWNER/REPO
//	    Set the project's field-notes repo on an already-initialized project.
//	    Persists to the project marker (per-project, committed).
//
//	ndf is-project
//	    Exit 0 if cwd (or $CLAUDE_PROJECT_DIR) is an NDF project,
//	    1 if absent, 2 on internal error. Silent on 0 and 1.
//
//	ndf marker-path
//	    Print the absolute resolved path to the project marker the CLI would
//	    consult (honors $CLAUDE_PROJECT_DIR). Does not check existence.
//
//	ndf config get <key> [--source]
//	    Print a single config value to stdout. Closed key set: version,
//	    pinned_version, fieldnotes_repo. Accepts kebab or snake form.
//	    --source prints the resolution source ("marker" or "legacy-config")
//	    to stderr.
//
//	ndf version   (aliases: --version, -v)
//	    Print the ndf CLI version. Inside an NDF project, also print the
//	    installed framework version on a second line. Human-facing readout;
//	    for a scripted framework-version read use `ndf config get version`.
//
// After a non-no-op update, ndf prints a team handoff message — a paste-ready
// block summarizing version bump, changes, and what coworkers need to do
// (git pull, merge main, /compact).
//
// Per-developer config:
//
//	Unix:    ~/.config/nandu/config.json   (mode 0600)
//	Windows: %APPDATA%\nandu\config.json
//
// Per-project marker: <project>/.ndf/cli/install.json
//
// Source of framework files: nandu-org/nandu-dev-framework  (private GitHub repo)
// Source of this CLI:        nandu-org/nandu-dev-framework-cli  (public)
package main

import (
	"os"
)

func main() {
	args := os.Args[1:]
	cmd := "help"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "init":
		cmdInit(args)
	case "login":
		cmdLogin(args)
	case "update":
		cmdUpdate(args)
	case "self-update":
		cmdSelfUpdate(args)
	case "config":
		cmdConfig(args)
	case "is-project":
		cmdIsProject(args)
	case "marker-path":
		cmdMarkerPath(args)
	case "version", "--version", "-v":
		cmdVersion(args)
	case "help", "--help", "-h":
		printHelp()
	default:
		printHelp()
		os.Exit(1)
	}
}
