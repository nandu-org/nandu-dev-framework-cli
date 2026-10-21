package main

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// legacySpecDir is the project-root folder every CLI before v2.10.0 delivered
// migration specs to, and where a manifest that names no migrations_dir still
// has them delivered. Many projects own that folder for their own database
// migrations, whose runners refuse a Markdown file — which is why the manifest
// can now say where specs go.
const legacySpecDir = "migrations"

// windowsVolume matches a drive-letter prefix (C:/…), which path.Clean does
// not treat as absolute but which is one on the platform that writes it.
var windowsVolume = regexp.MustCompile(`^[A-Za-z]:`)

// specDeliveryDir returns the project-relative directory this release's
// /ndf-migrate reads migration specs from: the manifest's migrations_dir when
// set, else legacySpecDir. Normalised to a clean, forward-slash, relative
// path. A manifest naming an absolute path (POSIX or drive-letter), the
// project root itself, or a path that escapes the project is a release
// defect: the error stops the update before anything is written.
func (m *Manifest) specDeliveryDir() (string, error) {
	raw := strings.TrimSpace(m.MigrationsDir)
	if raw == "" {
		return legacySpecDir, nil
	}
	d := path.Clean(strings.ReplaceAll(raw, "\\", "/"))
	if d == "." || strings.HasPrefix(d, "/") || windowsVolume.MatchString(d) || d == ".." || strings.HasPrefix(d, "../") {
		return "", fmt.Errorf("manifest migrations_dir %q must be a relative path inside the project", raw)
	}
	return d, nil
}

// validMigrationName reports whether a manifest migration name is a plain
// file basename: non-empty, no path separator, not a dot segment. The name
// becomes <dir>/<name>.md for delivery and removal, so a name with a
// separator or ".." would reach outside the delivery directory.
func validMigrationName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

// specDeliveryPath is the project-relative path a migration spec is
// delivered to: <dir>/<name>.md. Forward slashes, like every manifest path
// the CLI hands to os and git. Callers validate the name first.
func specDeliveryPath(dir, name string) string {
	return path.Join(dir, name+".md")
}

// tidyAppliedMigrationSpecs removes the spec file of every manifest migration
// whose sentinel is on disk, from the delivery directory and from
// legacySpecDir.
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
// Rules, each the positive form:
//   - Only names listed in the manifest are touched, never a glob, and a name
//     that is not a plain basename is skipped with a warning.
//   - A removal is recorded as a change only when git tracks the file: that
//     is what the commit offer can stage and what coworkers hold. An
//     untracked spec (a stopped run's leftover) is removed and not recorded.
//   - A directory is removed only when it is a real directory (never through
//     a symlink), this run removed a spec from it, and that left it empty.
//     A directory the project owns — one this run took nothing from, or one
//     that still holds the project's files — stays.
func tidyAppliedMigrationSpecs(m *Manifest, deliveryDir string, changes *[]change) {
	// The delivery dir the fetched manifest names, the CLI's own spec
	// folder (a stale spec can sit there after a pin to an older release
	// whose manifest does not name it), and the legacy project-root folder.
	dirs := []string{deliveryDir}
	for _, d := range []string{cliMigrationSpecsDir, legacySpecDir} {
		if d != deliveryDir {
			dirs = append(dirs, d)
		}
	}
	projDir := resolveProjectPath(".")
	inRepo := gitIsRepo(projDir)
	for _, dir := range dirs {
		dirAbs := resolveProjectPath(dir)
		st, err := os.Lstat(dirAbs)
		if err != nil || !st.IsDir() {
			// Absent, a file, or a symlink: nothing of ours to tidy, and a
			// symlink is never followed.
			continue
		}
		removedHere := 0
		for _, name := range m.Migrations {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if !validMigrationName(name) {
				warn("  manifest migration name %q is not a plain file name — leaving it alone.", name)
				continue
			}
			if !migrationSentinelExists(name) {
				continue
			}
			rel := specDeliveryPath(dir, name)
			abs := resolveProjectPath(rel)
			fst, err := os.Lstat(abs)
			if err != nil || !fst.Mode().IsRegular() {
				continue
			}
			tracked := false
			if inRepo {
				if _, err := gitInRepo(projDir, "ls-files", "--error-unmatch", "--", rel); err == nil {
					tracked = true
				}
			}
			if err := os.Remove(abs); err != nil {
				warn("  could not remove %s: %v", rel, err)
				continue
			}
			removedHere++
			info("  removed %s (migration already applied; nothing reads the spec)", rel)
			if tracked {
				*changes = append(*changes, change{Kind: changeRemove, Path: rel})
			}
		}
		if removedHere > 0 {
			removeDirIfEmptied(dirAbs)
		}
	}
}

// removeDirIfEmptied removes abs when it is a real, now-empty directory —
// called only for a directory this run removed a spec from. Anything else
// (a symlink, a file, a directory with content) is left alone.
func removeDirIfEmptied(abs string) {
	st, err := os.Lstat(abs)
	if err != nil || !st.IsDir() {
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil || len(entries) != 0 {
		return
	}
	_ = os.Remove(abs)
}
