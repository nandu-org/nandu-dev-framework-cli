// ndf — Nandu Development Framework CLI.
//
// Subcommands:
//
//	ndf init        [--token=<framework_pat>] [--fieldnotes-token=<fieldnotes_pat>]
//	                [--fieldnotes-repo=<owner/repo>] [--version=<x.y.z>]
//	    Scaffold a NEW NDF project. Refuses on existing .ndf.json — use
//	    `ndf login` to set tokens for an existing project.
//
//	ndf login       [--token=<framework_pat>] [--fieldnotes-token=<fieldnotes_pat>]
//	    Set per-developer credentials. Interactive by default (hidden input);
//	    accepts flags for non-interactive use (CI).
//
//	ndf update      [--version=<x.y.z>] [--latest]
//	    Update an existing NDF project to the target framework version.
//
//	ndf config show
//	    Print the resolved config with PATs masked.
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
// Per-project marker: <project>/.ndf.json
//
// Source of framework files: nandu-org/nandu-dev-framework  (private GitHub repo)
// Source of this CLI:        nandu-org/nandu-dev-framework-cli  (public)
package main

import (
	"fmt"
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
	case "config":
		cmdConfig(args)
	case "version", "--version", "-v":
		fmt.Println("ndf v" + CLIVersion)
	case "help", "--help", "-h":
		printHelp()
	default:
		printHelp()
		os.Exit(1)
	}
}
