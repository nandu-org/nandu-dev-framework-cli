package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// the six migrations registered in framework v4.16.0's manifest, in manifest
// order. Derived from `git show v4.16.0:manifest.json`, not hand-recalled.
var v4160Migrations = []string{
	"v3.1-to-v3.2-status-partition",
	"v3-to-v4-feature-scoped",
	"v4.0-to-v4.2-heavyweight-phases",
	"v4.3-to-v4.4-cli-state-relocation",
	"v4.5-to-v4.6-flatten-and-consolidate",
	"v4.15-to-v4.16-settings-split",
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what
// it wrote. The CLI's help/log helpers write to os.Stderr directly, so this is
// the only way to assert on them. Safe for help-sized output: the text is far
// under the pipe buffer, so a single synchronous read after the call cannot
// deadlock.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// chdirToTemp anchors project-path resolution at a scratch dir, the way
// `ndf init` anchors it at the project it is standing in. Manual Chdir +
// defer (not t.Chdir) to stay compatible with the module's declared go1.22.
func chdirToTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_PROJECT_DIR", "") // empty => resolveProjectPath falls back to cwd
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	return dir
}

// TestFreshInitWithoutSeedingLeavesEveryMigrationPending pins the C1 bug
// itself: `ndf init` seeds no sentinels, so a project that has never had
// anything to migrate reports every registered migration as pending. This is
// the red control — it asserts the broken behaviour the seeding fix removes,
// so a green result from the sibling test below cannot be vacuous.
func TestFreshInitWithoutSeedingLeavesEveryMigrationPending(t *testing.T) {
	chdirToTemp(t)
	m := &Manifest{Version: "4.16.0", Migrations: v4160Migrations}

	if got := pendingMigrationsFromManifest(m); len(got) != len(v4160Migrations) {
		t.Fatalf("precondition failed: want all %d pending on an unseeded fresh install, got %d (%v)",
			len(v4160Migrations), len(got), got)
	}
}

// TestFreshInitSeedsSentinelsSoNothingPends is the C1 regression test. A
// project created by `ndf init` is in the installed version's shape by
// construction, so every migration registered up to that version is vacuously
// satisfied and the gate must not fire at all.
func TestFreshInitSeedsSentinelsSoNothingPends(t *testing.T) {
	chdirToTemp(t)
	m := &Manifest{Version: "4.16.0", Migrations: v4160Migrations}

	if err := seedFreshInstallSentinels(m); err != nil {
		t.Fatalf("seedFreshInstallSentinels: %v", err)
	}

	if got := pendingMigrationsFromManifest(m); len(got) != 0 {
		t.Errorf("fresh install must have zero pending migrations, got %d: %v", len(got), got)
	}
}

// TestSeedFreshInstallSentinelsWritesToTheNewPath pins the write location.
// Sentinels written at the legacy `.ndf-migrations/` path would be read back
// fine (migrationSentinelExists is dual-path) but would trip
// v4.3-to-v4.4-cli-state-relocation's Step 0.5, which treats the presence of
// the old sentinel directory as evidence of a prior partial run (State E).
func TestSeedFreshInstallSentinelsWritesToTheNewPath(t *testing.T) {
	chdirToTemp(t)
	m := &Manifest{Version: "4.16.0", Migrations: v4160Migrations}

	if err := seedFreshInstallSentinels(m); err != nil {
		t.Fatalf("seedFreshInstallSentinels: %v", err)
	}

	for _, name := range v4160Migrations {
		if !fileExists(migrationSentinelPath(name)) {
			t.Errorf("no sentinel at the new path for %q", name)
		}
		if fileExists(oldMigrationSentinelPath(name)) {
			t.Errorf("sentinel for %q written at the LEGACY path; that state reads as "+
				"a prior partial run to the v4.3-to-v4.4 relocation spec", name)
		}
	}
}

// TestSeedFreshInstallSentinelsIsManifestDerived pins the all-or-nothing
// property against the manifest actually installed. `ndf init --version=` pins
// to an older framework, whose manifest registers fewer migrations; seeding
// must record exactly those, leaving later ones to run on catch-up.
func TestSeedFreshInstallSentinelsIsManifestDerived(t *testing.T) {
	chdirToTemp(t)

	// A pin at v4.6.0: its manifest registers the first five migrations.
	pinned := &Manifest{Version: "4.6.0", Migrations: v4160Migrations[:5]}
	if err := seedFreshInstallSentinels(pinned); err != nil {
		t.Fatalf("seedFreshInstallSentinels: %v", err)
	}
	if got := pendingMigrationsFromManifest(pinned); len(got) != 0 {
		t.Errorf("pinned fresh install must have zero pending, got %v", got)
	}

	// Later catch-up to v4.16.0 must leave exactly the unseeded sixth pending.
	latest := &Manifest{Version: "4.16.0", Migrations: v4160Migrations}
	got := pendingMigrationsFromManifest(latest)
	if len(got) != 1 || got[0] != "v4.15-to-v4.16-settings-split" {
		t.Errorf("catch-up must leave exactly the post-pin migration pending, got %v", got)
	}
}

// TestSeedFreshInstallSentinelsSkipsBlankNames mirrors
// pendingMigrationsFromManifest's own tolerance: it trims and skips empty
// entries, so seeding must not create a stray ".complete" file for one.
func TestSeedFreshInstallSentinelsSkipsBlankNames(t *testing.T) {
	chdirToTemp(t)
	m := &Manifest{Version: "4.16.0", Migrations: []string{"  ", "", " real-one "}}

	if err := seedFreshInstallSentinels(m); err != nil {
		t.Fatalf("seedFreshInstallSentinels: %v", err)
	}
	if got := pendingMigrationsFromManifest(m); len(got) != 0 {
		t.Errorf("blank entries must not pend, got %v", got)
	}
	entries, err := os.ReadDir(resolveProjectPath(migrationsSentinelDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("want exactly one sentinel for the one real name, got %d: %v", len(entries), names)
	}
}

// TestInitHelpDoesNotOfferVersionPinning pins the removal of `ndf init
// --version=`. `ndf update --version=<x.y.z>` already sets pinned_version and
// moves to it in one step, and is the path the README documents; a second route
// to the same outcome bought nothing and cost coherence, because this CLI
// writes state under .ndf/cli/ from birth while frameworks older than v4.4.0
// still expect it at the project root.
//
// The help text is the contract a user reads, so it is what this asserts. An
// unrecognised --version= reaches cmdInit's unknown-flag branch, which is the
// generic path every other stray flag takes.
func TestInitHelpDoesNotOfferVersionPinning(t *testing.T) {
	help := captureStderr(t, printHelpInit)

	if help == "" {
		t.Fatal("printHelpInit produced nothing; the assertions below would be vacuous")
	}
	// The help legitimately points at `ndf update --version=` as the route to an
	// older version, so assert that pointer is present, then strip it before
	// asserting init advertises no --version flag of its own.
	if !strings.Contains(help, "ndf update --version=") {
		t.Errorf("init help should point at `ndf update --version=` as the way to start on an older version:\n%s", help)
	}
	if rest := strings.ReplaceAll(help, "ndf update --version=", ""); strings.Contains(rest, "--version") {
		t.Errorf("ndf init help still advertises a --version flag of its own:\n%s", help)
	}
}

// TestHelpTextIsNotMangledByFormatVerbs guards every help surface against the
// bug that `ndf init --help` shipped through v2.8.1: rawErr passes its string
// to Fprintf as a *format*, so a literal "%APPDATA%\nandu" rendered as
// "%!A(MISSING)PPDATA%!\(MISSING)nandu" — garbage in the exact place a Windows
// user looks up where their config lives.
//
// The fix is to escape the percent (%%APPDATA%%); this test is what keeps the
// next literal % from re-breaking it. Asserting on "%!" catches every bad-verb
// shape Go emits (%!A(MISSING), %!(NOVERB), %!(EXTRA)) rather than just the one
// that bit us.
func TestHelpTextIsNotMangledByFormatVerbs(t *testing.T) {
	surfaces := map[string]func(){
		"init":        printHelpInit,
		"login":       printHelpLogin,
		"config":      printHelpConfig,
		"config show": printHelpConfigShow,
		"config get":  printHelpConfigGet,
		"config set":  printHelpConfigSet,
		"update":      printHelpUpdate,
		"self-update": printHelpSelfUpdate,
		"version":     printHelpVersion,
		"help":        printHelp,
	}
	for name, fn := range surfaces {
		out := captureStderr(t, fn)
		if out == "" {
			t.Errorf("%s: help printed nothing; the check below would be vacuous", name)
			continue
		}
		if strings.Contains(out, "%!") {
			t.Errorf("ndf %s --help mangles a literal %% through Fprintf:\n%s", name, out)
		}
	}
	// The line that actually regressed must render literally.
	if got := captureStderr(t, printHelpInit); !strings.Contains(got, `%APPDATA%\nandu\config.json`) {
		t.Errorf("init help should show the real Windows config path; got:\n%s", got)
	}
}

// TestSeedFreshInstallSentinelsContentNamesItself keeps the seeded file
// self-describing. Nothing reads sentinel contents (the CLI tests existence
// only, and no shipped spec reads a .complete file), so this is for the human
// who opens one — it must not read as a record that a migration actually ran.
func TestSeedFreshInstallSentinelsContentNamesItself(t *testing.T) {
	chdirToTemp(t)
	m := &Manifest{Version: "4.16.0", Migrations: []string{"v3.1-to-v3.2-status-partition"}}

	if err := seedFreshInstallSentinels(m); err != nil {
		t.Fatalf("seedFreshInstallSentinels: %v", err)
	}
	b, err := os.ReadFile(migrationSentinelPath("v3.1-to-v3.2-status-partition"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if !strings.Contains(body, "ndf init") || !strings.Contains(body, "4.16.0") {
		t.Errorf("seeded sentinel should say it came from `ndf init` at the installed version; got:\n%s", body)
	}
}
