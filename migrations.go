package main

import (
	"fmt"
	"os"
	"path"
	"strings"
)

// legacySpecDir is the project-root folder every CLI before v2.10.0 delivered
// migration specs to, and where a manifest that names no migrations_dir still
// has them delivered. Many projects own that folder for their own database
// migrations, whose runners refuse a Markdown file — which is why the manifest
// can now say where specs go.
const legacySpecDir = "migrations"

// specDeliveryDir returns the project-relative directory this release's
// /ndf-migrate reads migration specs from: the manifest's migrations_dir when
// set, else legacySpecDir. Normalised to a clean, forward-slash, relative
// path. A manifest naming an absolute path, the project root itself, or a
// path that escapes the project is a release defect: the error stops the
// update before anything is written.
func (m *Manifest) specDeliveryDir() (string, error) {
	raw := strings.TrimSpace(m.MigrationsDir)
	if raw == "" {
		return legacySpecDir, nil
	}
	d := path.Clean(strings.ReplaceAll(raw, "\\", "/"))
	if d == "." || strings.HasPrefix(d, "/") || d == ".." || strings.HasPrefix(d, "../") {
		return "", fmt.Errorf("manifest migrations_dir %q must be a relative path inside the project", raw)
	}
	return d, nil
}

// specDeliveryPath is the project-relative path a migration spec is
// delivered to: <dir>/<name>.md. Forward slashes, like every manifest path
// the CLI hands to os and git.
func specDeliveryPath(dir, name string) string {
	return path.Join(dir, name+".md")
}

// tidyAppliedMigrationSpecs removes the spec file of every manifest migration
// whose sentinel is on disk, from the delivery directory and from
// legacySpecDir, then removes either directory if that left it empty.
//
// Called on the update path where every listed migration is already applied.
// A spec still on disk there was delivered by a CLI before v2.10.0 (which
// never removed specs; projects committed them) or left by a /ndf-migrate
// run that stopped between its sentinel write and its own cleanup. Nothing
// reads a spec once its sentinel exists — the gate keys on sentinels and a
// catch-up client fetches any pending spec from the release — so the file is
// dead weight, and in a project that owns migrations/ for its own runner it
// is a broken build.
//
// Only names listed in the manifest are touched, never a glob, and a
// directory holding anything else stays. Each removal is recorded as a
// change so it reaches the commit offer and the team handoff.
func tidyAppliedMigrationSpecs(m *Manifest, deliveryDir string, changes *[]change) {
	dirs := []string{deliveryDir}
	if deliveryDir != legacySpecDir {
		dirs = append(dirs, legacySpecDir)
	}
	for _, dir := range dirs {
		for _, name := range m.Migrations {
			name = strings.TrimSpace(name)
			if name == "" || !migrationSentinelExists(name) {
				continue
			}
			rel := specDeliveryPath(dir, name)
			abs := resolveProjectPath(rel)
			st, err := os.Stat(abs)
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
			if err := os.Remove(abs); err != nil {
				warn("  could not remove %s: %v", rel, err)
				continue
			}
			info("  removed %s (migration already applied; nothing reads the spec)", rel)
			*changes = append(*changes, change{Kind: changeRemove, Path: rel})
		}
		removeDirIfEmpty(resolveProjectPath(dir))
	}
}

// removeDirIfEmpty removes abs when it is an empty directory; anything else
// (absent, a file, a directory with content) is left alone.
func removeDirIfEmpty(abs string) {
	entries, err := os.ReadDir(abs)
	if err != nil || len(entries) != 0 {
		return
	}
	_ = os.Remove(abs)
}
