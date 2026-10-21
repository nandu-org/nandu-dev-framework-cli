package main

import (
	"os"
	"path/filepath"
	"reflect"
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
	_, err := os.Stat(rel)
	return err == nil
}

// TestTidyAppliedMigrationSpecs is the synthetic reproduction of the leftover
// state the field found: specs of applied migrations committed in the
// project's own migrations/ folder by earlier CLIs, next to the project's
// database migrations. The tidy must remove exactly the applied specs, in
// both delivery locations, leave the project's files and any pending spec,
// remove a directory only when empty, record each removal, and be a no-op
// the second time.
func TestTidyAppliedMigrationSpecs(t *testing.T) {
	chdirToTemp(t)
	for _, n := range []string{"a", "b"} {
		writeFile(t, ".ndf/cli/sentinels/"+n+".complete", "done\n")
	}
	writeFile(t, "migrations/a.md", "spec a")
	writeFile(t, "migrations/b.md", "spec b")
	writeFile(t, "migrations/c.md", "spec c (pending — no sentinel)")
	writeFile(t, "migrations/001_feedback.sql", "create table feedback();")
	writeFile(t, ".ndf/cli/migrations/a.md", "spec a")
	writeFile(t, ".ndf/cli/migrations/c.md", "spec c")
	m := &Manifest{Migrations: []string{"a", "b", "c", " "}}

	var changes []change
	tidyAppliedMigrationSpecs(m, ".ndf/cli/migrations", &changes)

	for _, gone := range []string{"migrations/a.md", "migrations/b.md", ".ndf/cli/migrations/a.md"} {
		if exists(gone) {
			t.Errorf("%s should have been removed", gone)
		}
	}
	for _, kept := range []string{"migrations/c.md", "migrations/001_feedback.sql", ".ndf/cli/migrations/c.md", "migrations", ".ndf/cli/migrations"} {
		if !exists(kept) {
			t.Errorf("%s should have been left alone", kept)
		}
	}
	want := []change{
		{Kind: changeRemove, Path: ".ndf/cli/migrations/a.md"},
		{Kind: changeRemove, Path: "migrations/a.md"},
		{Kind: changeRemove, Path: "migrations/b.md"},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Errorf("changes = %v, want %v", changes, want)
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
		t.Errorf(".ndf/cli/migrations should be removed once empty")
	}
	if exists("migrations/c.md") || !exists("migrations/001_feedback.sql") || !exists("migrations") {
		t.Errorf("migrations/: c.md removed=%v sql kept=%v dir kept=%v", !exists("migrations/c.md"), exists("migrations/001_feedback.sql"), exists("migrations"))
	}
	if len(third) != 2 {
		t.Errorf("third run recorded %v, want two removals", third)
	}
}

// With no migrations_dir the delivery dir IS the legacy dir: one pass, and an
// emptied migrations/ created by spec delivery alone is removed.
func TestTidyAppliedMigrationSpecsLegacyOnly(t *testing.T) {
	chdirToTemp(t)
	writeFile(t, ".ndf/cli/sentinels/a.complete", "done\n")
	writeFile(t, "migrations/a.md", "spec a")
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
