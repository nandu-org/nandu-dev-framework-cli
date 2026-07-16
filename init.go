package main

import (
	"fmt"
	"os"
	"strings"
	"time"
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

// seedFreshInstallSentinels records every migration registered in the manifest
// `init` just installed as already satisfied.
//
// A project created by `ndf init` is in the installed version's shape by
// construction, so every migration registered up to that version is vacuously
// complete — there has never been anything for them to migrate. Recording that
// is not a shortcut around the migration system; it is the truthful history of
// a project born at this version.
//
// Without the record, pendingMigrationsFromManifest reports all of them pending
// on the very first `ndf update`, /ndf-migrate runs them in manifest order, and
// the first one reads planning artifacts (docs/plan/decisions.md) that no
// framework version has ever shipped and a fresh project has never created. It
// halts, so no sentinel is written, so the gate re-fires identically on every
// subsequent update — permanently. The v4.5-to-v4.6 spec halts on the same
// class of absent input one step further on.
//
// Seeding the manifest whole — never a subset — is load-bearing rather than
// tidy. v4.3-to-v4.4-cli-state-relocation's Step 0.5 reads "any sentinel other
// than my own at the new path" as evidence of a prior aborted run (State E →
// halt), so a partial record leaves that migration pending and hands it
// precisely that state. `init` always installs the latest tag, whose migration
// set is a superset of every earlier one, so the whole-manifest record keeps it
// out of the pending set on every later `ndf update` — including one pinned
// backwards via `ndf update --version=`, whose migration set is a prefix of
// what was seeded here.
//
// Sentinels are written only at the new path: the legacy `.ndf-migrations/`
// directory is itself a State E signal to the same spec. Nothing anywhere reads
// their contents — the CLI tests existence only, and no shipped spec reads a
// .complete file — so the body is for whoever opens one by hand.
func seedFreshInstallSentinels(m *Manifest) error {
	dir := resolveProjectPath(migrationsSentinelDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", migrationsSentinelDir, err)
	}
	body := fmt.Sprintf(
		"seeded_at: %s\nseeded_by: ndf init (CLI v%s)\nreason: fresh install at framework v%s — no pre-existing state to migrate\n",
		time.Now().UTC().Format("2006-01-02T15:04:05Z"), CLIVersion, m.Version,
	)
	for _, name := range m.Migrations {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if err := os.WriteFile(migrationSentinelPath(name), []byte(body), 0o644); err != nil {
			return fmt.Errorf("write sentinel for %s: %w", name, err)
		}
	}
	return nil
}

// cmdInit is the entry point for `ndf init`.
func cmdInit(args []string) {
	var (
		cliFrameworkPAT   string
		cliFieldnotesPAT  string
		cliFieldnotesRepo string
	)

	// No --version flag: `ndf init` always installs the latest tag. Starting a
	// project on an older framework is `ndf update --version=<x.y.z>`, which
	// sets pinned_version and moves to it in one step — the path the README
	// documents. A second route to the same outcome bought nothing and cost
	// coherence: this CLI writes project state under .ndf/cli/ from birth, so
	// initialising a framework older than v4.4.0 (the release whose relocation
	// migration creates that layout) produced a project whose own migration set
	// expected state where the CLI never puts it. An unrecognised --version=
	// now falls to the unknown-flag branch below.
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--token="):
			cliFrameworkPAT = strings.TrimPrefix(a, "--token=")
		case strings.HasPrefix(a, "--fieldnotes-token="):
			cliFieldnotesPAT = strings.TrimPrefix(a, "--fieldnotes-token=")
		case strings.HasPrefix(a, "--fieldnotes-repo="):
			cliFieldnotesRepo = strings.TrimPrefix(a, "--fieldnotes-repo=")
			if err := validateRepoSlug(cliFieldnotesRepo); err != nil {
				die("invalid --fieldnotes-repo: %v", err)
			}
		case a == "-h" || a == "--help":
			printHelpInit()
			return
		default:
			die("unknown init flag: %s", a)
		}
	}

	// Write command: scaffold into the current directory. See anchorProjectToCwd
	// — the existence check, the framework files, and the marker all resolve to
	// cwd, so init creates (and refuses on) exactly the project you're standing
	// in, never one directory named by $CLAUDE_PROJECT_DIR while writing into
	// another.
	anchorProjectToCwd()

	// Strict: refuse on existing project. Direct user to the right command.
	// Dual-path check: a project may have its marker at either the
	// v2.5.0+ location (.ndf/cli/install.json) or the pre-v2.5.0
	// location (.ndf.json at the project root). Either presence means
	// `ndf init` should refuse; the user is in an already-NDF project
	// and should use `ndf update` (or `ndf login` for credentials).
	newPath := markerPath()
	oldPath := oldMarkerPath()
	newExists := fileExists(newPath)
	oldExists := fileExists(oldPath)
	if newExists || oldExists {
		foundPath := newPath
		if !newExists {
			foundPath = oldPath
		}
		die("ndf project state found at %s. To update the project: `ndf update`. To set credentials: `ndf login`.", foundPath)
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

	// If --fieldnotes-repo was omitted, prompt for it interactively.
	// Mirrors how `ndf login` prompts for missing tokens. CI / non-TTY:
	// prompt() returns the default ("") and we fall through to the
	// warn-and-continue path at the end of init (some clients legitimately
	// don't have a field-notes repo provisioned at init time).
	if cliFieldnotesRepo == "" {
		cliFieldnotesRepo = prompt(
			"Field-notes repo (e.g. nandu-org/Example-FieldNotes) — leave empty to skip:",
			"",
		)
		if cliFieldnotesRepo != "" {
			if err := validateRepoSlug(cliFieldnotesRepo); err != nil {
				die("invalid fieldnotes_repo from prompt: %v", err)
			}
		}
	}

	// Empty version => resolveRef picks the latest semver tag.
	ref, err := resolveRef("")
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

	// Never pinned at init: a fresh project starts on the latest tag and
	// tracks it. `ndf update --version=<x.y.z>` is what sets pinned_version.
	m := &Marker{
		Version:            manifest.Version,
		PinnedVersion:      nil,
		InstalledChecksums: checksums,
		FieldnotesRepo:     cliFieldnotesRepo,
	}
	// Create the CLI-state directory tree (.ndf/cli/sentinels/) before
	// writeMarker so the rename target's parent exists. writeMarker
	// itself also MkdirAlls .ndf/cli/ as belt-and-suspenders.
	//
	// Seeding fills that directory rather than leaving it empty: this project
	// is in v<manifest.Version> shape from birth, so every migration the
	// manifest registers is already satisfied. Note the sentinels are
	// deliberately NOT added to `checksums` above — they are CLI state, not
	// framework files, and an entry in installed_checksums would make the next
	// `ndf update`'s removed-files pass delete the sentinel and re-fire the
	// migration gate forever.
	if err := seedFreshInstallSentinels(manifest); err != nil {
		die("%v", err)
	}
	if err := writeMarker(m); err != nil {
		die("write marker: %v", err)
	}

	cwd, _ := os.Getwd()
	ok("ndf init complete. Installed v%s into %s.", manifest.Version, cwd)
	if cliFieldnotesRepo != "" {
		info("fieldnotes_repo set to %s in %s — commit this so coworkers pick it up automatically.", cliFieldnotesRepo, projectMarker)
	} else {
		warn("no --fieldnotes-repo provided; /field-note won't have a target until it's set in %s (or fall back to ~/.config/nandu/config.json).", projectMarker)
	}
	ok("Next steps: edit CLAUDE.project.md, .claude/hooks/pre-commit-tests.sh, and .claude/settings.json (allow-list) per METHODOLOGY.md.")
}
