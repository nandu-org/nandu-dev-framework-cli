package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSpecDeliveryDir pins the manifest → delivery-directory contract: absent
// or blank falls back to the project-root migrations/ folder every earlier
// release used; a set value is cleaned; anything that is not a relative path
// inside the project is a release defect and errors.
func TestSpecDeliveryDir(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "migrations", false},
		{"   ", "migrations", false},
		{".ndf/cli/migrations", ".ndf/cli/migrations", false},
		{".ndf/cli/migrations/", ".ndf/cli/migrations", false},
		{"./.ndf/cli/migrations", ".ndf/cli/migrations", false},
		{".ndf\\cli\\migrations", ".ndf/cli/migrations", false},
		{"/abs/migrations", "", true},
		{"C:\\x", "", true},
		{"c:/x/y", "", true},
		{"../outside", "", true},
		{"a/../../outside", "", true},
		{".", "", true},
		{"./", "", true},
	}
	for _, c := range cases {
		m := &Manifest{MigrationsDir: c.in}
		got, err := m.specDeliveryDir()
		if c.wantErr {
			if err == nil {
				t.Errorf("specDeliveryDir(%q) = %q, want error", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("specDeliveryDir(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestValidMigrationName(t *testing.T) {
	for _, ok := range []string{"v3-to-v4-feature-scoped", "v4.29-to-v5.0-system-description", "x"} {
		if !validMigrationName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "a/b", `a\b`, "../../../../etc/x"} {
		if validMigrationName(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestSpecDeliveryPath(t *testing.T) {
	if got := specDeliveryPath("migrations", "v3-to-v4-feature-scoped"); got != "migrations/v3-to-v4-feature-scoped.md" {
		t.Errorf("got %q", got)
	}
	if got := specDeliveryPath(".ndf/cli/migrations", "v4.29-to-v5.0-system-description"); got != ".ndf/cli/migrations/v4.29-to-v5.0-system-description.md" {
		t.Errorf("got %q", got)
	}
}

func writeFile(t *testing.T, rel string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rel, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(rel string) bool {
	_, err := os.Lstat(rel)
	return err == nil
}

// gitInit turns the current directory into a repository with one commit of
// the given paths, so the tidy's tracked/untracked distinction can be tested.
func gitInit(t *testing.T, trackedPaths ...string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if len(trackedPaths) > 0 {
		run(append([]string{"add", "--"}, trackedPaths...)...)
		run("commit", "-q", "-m", "fixture")
	}
}

// TestTidyAppliedMigrationSpecs is the synthetic reproduction of the leftover
// state the field found: specs of applied migrations committed in the
// project's own migrations/ folder by earlier CLIs, next to the project's
// database migrations, plus an untracked spec a stopped run left in the
// delivery folder. The tidy must remove exactly the applied specs in both
// locations, record only the tracked ones (the printed git add must be
// runnable), leave the project's files and any pending spec, remove a folder
// only when it emptied it, and be a no-op the second time.
func TestTidyAppliedMigrationSpecs(t *testing.T) {
	chdirToTemp(t)
	for _, n := range []string{"a", "b"} {
		writeFile(t, ".ndf/cli/sentinels/"+n+".complete", "done\n")
	}
	writeFile(t, "migrations/a.md", "spec a")
	writeFile(t, "migrations/b.md", "spec b")
	writeFile(t, "migrations/c.md", "spec c (pending — no sentinel)")
	writeFile(t, "migrations/001_feedback.sql", "create table feedback();")
	writeFile(t, ".ndf/cli/migrations/a.md", "spec a (untracked leftover)")
	writeFile(t, ".ndf/cli/migrations/c.md", "spec c")
	// A defective manifest name that resolves outside the folder: its
	// "sentinel" and "spec" both exist where the unvalidated path would
	// land, so only the name check keeps evil.md alive.
	writeFile(t, ".ndf/cli/evil.complete", "done\n")
	writeFile(t, "evil.md", "project file that a traversal name would hit")
	gitInit(t, "migrations/a.md", "migrations/b.md", "migrations/001_feedback.sql", ".ndf/cli/sentinels")
	m := &Manifest{Migrations: []string{"a", "b", "c", " ", "../evil"}}

	var changes []change
	tidyAppliedMigrationSpecs(m, ".ndf/cli/migrations", &changes)

	for _, gone := range []string{"migrations/a.md", "migrations/b.md", ".ndf/cli/migrations/a.md"} {
		if exists(gone) {
			t.Errorf("%s should have been removed", gone)
		}
	}
	for _, kept := range []string{"migrations/c.md", "migrations/001_feedback.sql", ".ndf/cli/migrations/c.md", "migrations", ".ndf/cli/migrations", "evil.md"} {
		if !exists(kept) {
			t.Errorf("%s should have been left alone", kept)
		}
	}
	// Only the two tracked removals are recorded: the untracked delivery-dir
	// leftover has nothing in git to stage and must not reach the printed
	// `git add` or the team handoff.
	want := []change{
		{Kind: changeRemove, Path: "migrations/a.md"},
		{Kind: changeRemove, Path: "migrations/b.md"},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Errorf("changes = %v, want %v", changes, want)
	}
	// The printed/staged pathspec must be runnable as-is. (The `.ndf/cli`
	// entry sweeps the still-pending c.md into the index here; unstage it
	// again so the later runs see it as the untracked file it is in a real
	// gate-fired project, where no commit offer runs.)
	stage := stagePathsForCommit(changes)
	cmd := exec.Command("git", append([]string{"add", "--"}, stage...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("git add of the recorded paths failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "reset", "-q", "--", ".ndf/cli/migrations").CombinedOutput(); err != nil {
		t.Fatalf("git reset: %v\n%s", err, out)
	}

	// Second run: nothing left to do, nothing recorded.
	var again []change
	tidyAppliedMigrationSpecs(m, ".ndf/cli/migrations", &again)
	if len(again) != 0 {
		t.Errorf("second run recorded %v", again)
	}

	// c completes: its specs go, the delivery dir is now empty and goes
	// too; migrations/ keeps the project's SQL and stays.
	writeFile(t, ".ndf/cli/sentinels/c.complete", "done\n")
	var third []change
	tidyAppliedMigrationSpecs(m, ".ndf/cli/migrations", &third)
	if exists(".ndf/cli/migrations") {
		t.Errorf(".ndf/cli/migrations should be removed once this run emptied it")
	}
	if exists("migrations/c.md") || !exists("migrations/001_feedback.sql") || !exists("migrations") {
		t.Errorf("migrations/: c.md removed=%v sql kept=%v dir kept=%v", !exists("migrations/c.md"), exists("migrations/001_feedback.sql"), exists("migrations"))
	}
	if len(third) != 0 {
		t.Errorf("third run recorded %v for untracked files", third)
	}
}

// With no migrations_dir the delivery dir IS the legacy dir: one pass, and an
// emptied migrations/ created by spec delivery alone is removed.
func TestTidyAppliedMigrationSpecsLegacyOnly(t *testing.T) {
	chdirToTemp(t)
	writeFile(t, ".ndf/cli/sentinels/a.complete", "done\n")
	writeFile(t, "migrations/a.md", "spec a")
	gitInit(t, "migrations/a.md")
	m := &Manifest{Migrations: []string{"a"}}
	var changes []change
	tidyAppliedMigrationSpecs(m, "migrations", &changes)
	if exists("migrations") {
		t.Errorf("emptied migrations/ should be removed")
	}
	if len(changes) != 1 || changes[0].Path != "migrations/a.md" {
		t.Errorf("changes = %v", changes)
	}
}

// A sentinel at the legacy root location (.ndf-migrations/) counts as applied
// too — the dual-path read the gate uses is the one the tidy must use.
func TestTidyHonoursLegacySentinelPath(t *testing.T) {
	chdirToTemp(t)
	writeFile(t, ".ndf-migrations/a.complete", "done\n")
	writeFile(t, "migrations/a.md", "spec a")
	var changes []change
	tidyAppliedMigrationSpecs(&Manifest{Migrations: []string{"a"}}, ".ndf/cli/migrations", &changes)
	if exists("migrations/a.md") {
		t.Errorf("spec of a migration applied under the legacy sentinel path should be removed")
	}
}

// A folder the project owns is never removed: one the tidy took nothing from
// (already empty), one that is a regular file, and one that is a symlink —
// which the tidy neither follows nor deletes, so a spec behind it also stays.
func TestTidyLeavesProjectOwnedFolders(t *testing.T) {
	chdirToTemp(t)
	writeFile(t, ".ndf/cli/sentinels/a.complete", "done\n")
	m := &Manifest{Migrations: []string{"a"}}

	// Already-empty project folder: untouched.
	if err := os.MkdirAll("migrations", 0o755); err != nil {
		t.Fatal(err)
	}
	var changes []change
	tidyAppliedMigrationSpecs(m, ".ndf/cli/migrations", &changes)
	if !exists("migrations") || len(changes) != 0 {
		t.Errorf("an empty project-owned migrations/ must stay (exists=%v changes=%v)", exists("migrations"), changes)
	}

	// A regular file at the folder's path: untouched.
	if err := os.Remove("migrations"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "migrations", "not a directory")
	tidyAppliedMigrationSpecs(m, ".ndf/cli/migrations", &changes)
	if st, err := os.Lstat("migrations"); err != nil || !st.Mode().IsRegular() {
		t.Errorf("a regular file named migrations must stay")
	}

	// A symlinked folder: never followed, never deleted; the spec behind it stays.
	if err := os.Remove("migrations"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "db/migrations/a.md", "spec a behind a link")
	if err := os.Symlink("db/migrations", "migrations"); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	tidyAppliedMigrationSpecs(m, ".ndf/cli/migrations", &changes)
	st, err := os.Lstat("migrations")
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the project's migrations symlink must stay")
	}
	if !exists("db/migrations/a.md") {
		t.Errorf("a spec behind a symlink must not be removed through it")
	}
	if len(changes) != 0 {
		t.Errorf("nothing should be recorded, got %v", changes)
	}
}

// The `.ndf/cli` stage entry is handed to git with the spec folder excluded:
// sentinels and the marker are staged, a pending or stale spec never is, and
// a tracked spec's recorded removal still stages through its own path.
func TestCliStatePathspecKeepsSpecsOut(t *testing.T) {
	chdirToTemp(t)
	writeFile(t, ".ndf/cli/install.json", "{}")
	writeFile(t, ".ndf/cli/migrations/old.md", "tracked spec")
	gitInit(t, ".ndf/cli/install.json", ".ndf/cli/migrations/old.md")
	writeFile(t, ".ndf/cli/sentinels/x.complete", "done\n")
	writeFile(t, ".ndf/cli/migrations/x.md", "pending spec")
	if err := os.Remove(".ndf/cli/migrations/old.md"); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run(append([]string{"add", "--"}, cliStatePathspec()...)...)
	st := run("status", "--porcelain")
	if !strings.Contains(st, "A  .ndf/cli/sentinels/x.complete") {
		t.Errorf("sentinel not staged:\n%s", st)
	}
	if strings.Contains(st, "A  .ndf/cli/migrations/x.md") {
		t.Errorf("pending spec must not be staged:\n%s", st)
	}
	if !strings.Contains(st, " D .ndf/cli/migrations/old.md") {
		t.Errorf("the excluded folder's tracked deletion is not staged by the folder entry (expected; it is staged by its own recorded path):\n%s", st)
	}
	// The recorded removal path stages on its own, as the commit loop does.
	run("add", "--", ".ndf/cli/migrations/old.md")
	if st := run("status", "--porcelain"); !strings.Contains(st, "D  .ndf/cli/migrations/old.md") {
		t.Errorf("tracked spec removal should stage through its own path:\n%s", st)
	}
}

// With a manifest that names no migrations_dir, the CLI's own spec folder is
// still swept: a stale spec left there (after a pin to an older release)
// goes, while the project's legacy folder is handled as before.
func TestTidySweepsCliFolderWithoutManifestField(t *testing.T) {
	chdirToTemp(t)
	writeFile(t, ".ndf/cli/sentinels/a.complete", "done\n")
	writeFile(t, ".ndf/cli/migrations/a.md", "stale spec")
	var changes []change
	tidyAppliedMigrationSpecs(&Manifest{Migrations: []string{"a"}}, "migrations", &changes)
	if exists(".ndf/cli/migrations/a.md") || exists(".ndf/cli/migrations") {
		t.Errorf("stale spec in the CLI-owned folder should be removed and the emptied folder with it")
	}
	if len(changes) != 0 {
		t.Errorf("untracked removal must not be recorded, got %v", changes)
	}
}
