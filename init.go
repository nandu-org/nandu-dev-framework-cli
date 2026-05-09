package main

import (
	"os"
	"strings"
)

// claudeProjectStub is the project-owned CLAUDE.project.md scaffold written
// by `ndf init` if the file isn't already present. It is NOT in the manifest
// — once written, the framework never touches it again. The client owns it
// forever.
const claudeProjectStub = `# <project-name>

## Stack
- Language: <e.g. Python 3.12, Node.js 20, Rust 1.75>
- Framework: <e.g. FastAPI, Express, Actix>
- Database: <e.g. BigQuery, Postgres, SQLite>
- Dependency cache path: <e.g. ` + "`node_modules/`" + `, ` + "`.venv/lib/python3.12/site-packages/`" + `, ` + "`~/.cargo/registry/src/`" + `> — used by the inspect-over-execute rule for ` + "`Read`" + `/` + "`Grep`" + ` on installed library source.
- <Other critical dependencies>

## Key rules
- <non-negotiable constraint — e.g. "all data access must be tenant-scoped">

## Verification commands
` + "```bash" + `
<test command>   # unit tests
<lint command>   # lint
<type command>   # type check
` + "```" + `

## Required tooling
<MCPs your project depends on; e.g. context7 for stack documentation>
`

// cmdInit is the entry point for `ndf init`.
func cmdInit(args []string) {
	var (
		cliFrameworkPAT  string
		cliFieldnotesPAT string
		cliFieldnotesRepo string
		requestedVersion string
	)

	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--token="):
			cliFrameworkPAT = strings.TrimPrefix(a, "--token=")
		case strings.HasPrefix(a, "--fieldnotes-token="):
			cliFieldnotesPAT = strings.TrimPrefix(a, "--fieldnotes-token=")
		case strings.HasPrefix(a, "--fieldnotes-repo="):
			cliFieldnotesRepo = strings.TrimPrefix(a, "--fieldnotes-repo=")
		case strings.HasPrefix(a, "--version="):
			requestedVersion = strings.TrimPrefix(a, "--version=")
		case a == "-h" || a == "--help":
			printHelpInit()
			return
		default:
			die("unknown init flag: %s", a)
		}
	}

	// Strict: refuse on existing project. Direct user to the right command.
	if _, err := os.Stat(projectMarker); err == nil {
		die("%s already exists. This is already an NDF project.\n\n  To set or update your credentials:  ndf login\n  To update the project:              ndf update", projectMarker)
	}

	// If tokens were passed as flags, persist them BEFORE the rest of init.
	// That way a partial scaffold still leaves credentials configured.
	if cliFrameworkPAT != "" || cliFieldnotesPAT != "" {
		c, err := loadConfig()
		if err != nil {
			die("%v", err)
		}
		if cliFrameworkPAT != "" {
			c.FrameworkPAT = cliFrameworkPAT
		}
		if cliFieldnotesPAT != "" {
			c.FieldnotesPAT = cliFieldnotesPAT
		}
		if err := saveConfig(c); err != nil {
			die("save config: %v", err)
		}
		info("tokens saved to %s (mode 0600)", configFile())
	}

	if resolveToken() == "" {
		die("no framework PAT configured. Run `ndf login` first, or pass --token=<ghp_xxx>.")
	}

	if resolveFieldnotesToken() == "" {
		warn("no field-notes PAT configured. /field-note will not work until you run `ndf login` with the field-notes token.")
	}

	ref, err := resolveRef(requestedVersion)
	if err != nil {
		die("resolve target ref: %v", err)
	}
	info("fetching manifest for %s…", ref)

	manifest, err := fetchManifest(ref)
	if err != nil {
		die("fetch manifest: %v", err)
	}
	checkMinCLIVersion(manifest)

	info("installing v%s", manifest.Version)

	// Fetch every file in the manifest and verify checksums on the way down.
	checksums := make(map[string]string, len(manifest.Files))
	for _, f := range manifest.Files {
		info("  %s", f.Path)
		if err := fetchFileTo(ref, f.Path, f.Path); err != nil {
			die("fetch %s: %v", f.Path, err)
		}
		got, err := sha256File(f.Path)
		if err != nil {
			die("checksum %s: %v", f.Path, err)
		}
		if got != f.Checksum {
			die("checksum mismatch on %s: expected %s, got %s. Aborting init; rerun or escalate.", f.Path, f.Checksum, got)
		}
		checksums[f.Path] = got
	}

	// Make hook script executable if present (chmod is a no-op on Windows
	// filesystems but harmless).
	if _, err := os.Stat(".claude/hooks/pre-commit-tests.sh"); err == nil {
		_ = os.Chmod(".claude/hooks/pre-commit-tests.sh", 0o755)
	}

	// Write the project-owned CLAUDE.project.md stub if not already present.
	if _, err := os.Stat("CLAUDE.project.md"); os.IsNotExist(err) {
		if err := os.WriteFile("CLAUDE.project.md", []byte(claudeProjectStub), 0o644); err != nil {
			warn("could not write CLAUDE.project.md stub: %v", err)
		} else {
			info("created CLAUDE.project.md stub (you own this file — fill it in)")
		}
	}

	// Pin if --version was passed.
	var pinned *string
	if requestedVersion != "" {
		pv := manifest.Version
		pinned = &pv
	}

	m := &Marker{
		Version:            manifest.Version,
		PinnedVersion:      pinned,
		InstalledChecksums: checksums,
		FieldnotesRepo:     cliFieldnotesRepo,
	}
	if err := writeMarker(m); err != nil {
		die("write marker: %v", err)
	}

	cwd, _ := os.Getwd()
	ok("ndf init complete. Installed v%s into %s.", manifest.Version, cwd)
	if cliFieldnotesRepo != "" {
		info("fieldnotes_repo set to %s in .ndf.json — commit this so coworkers pick it up automatically.", cliFieldnotesRepo)
	} else {
		warn("no --fieldnotes-repo provided; /field-note won't have a target until it's set in .ndf.json (or fall back to ~/.config/nandu/config.json).")
	}
	ok("Next steps: edit CLAUDE.project.md, .claude/hooks/pre-commit-tests.sh, and .claude/settings.json (allow-list) per METHODOLOGY.md.")
}
