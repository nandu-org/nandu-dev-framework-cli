package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Marker is the per-project pinning + checksum file (.ndf.json).
//
// Schema:
//
//	{
//	  "version":             "4.0.0",
//	  "pinned_version":      null | "4.0.0",
//	  "installed_checksums": { "<path>": "<sha256>", ... },
//	  "fieldnotes_repo":     "owner/repo"   // optional
//	}
//
// History: v2.2.0 added a project-identity field that drove
// migration-companion file routing (a canary map keyed off the tag,
// delivered by the CLI alongside the migration spec). v2.3.1 removed
// the field cleanly — the canary-map mechanism is now self-authored by
// the migration spec at /ndf-migrate time, so the CLI no longer needs
// project identity in the marker. Old on-disk markers that still
// carry the field are tolerated (JSON unmarshal ignores unknown
// fields) and the field drops off on the next rewrite via writeMarker.
type Marker struct {
	Version            string            `json:"version"`
	PinnedVersion      *string           `json:"pinned_version"` // null when not pinned
	InstalledChecksums map[string]string `json:"installed_checksums"`
	FieldnotesRepo     string            `json:"fieldnotes_repo,omitempty"`
}

// loadMarker reads the project marker. Tries the v2.5.0+ path first
// (.ndf/cli/install.json); on os.IsNotExist falls back to the pre-v2.5.0
// path (.ndf.json at the project root). nil + nil if neither exists.
// Empty / malformed file returns an error referencing whichever path
// actually failed.
func loadMarker() (*Marker, error) {
	m, _, err := loadMarkerWithSource()
	return m, err
}

// loadMarkerWithSource is the source-aware variant of loadMarker: the
// second return value is "new" if the marker was read from
// markerPath() or "old" if it was read from the dual-path fallback at
// oldMarkerPath(). Empty string if absent (m == nil). No current
// callers consume the source, but the helper enables future call sites
// (e.g., warning when a write would diverge from the read location).
func loadMarkerWithSource() (*Marker, string, error) {
	// NEW path first.
	if m, err := readMarkerFile(markerPath()); err != nil || m != nil {
		if err != nil {
			return nil, "", err
		}
		return m, "new", nil
	}
	// Fall back to the OLD location (catch-up window).
	if m, err := readMarkerFile(oldMarkerPath()); err != nil || m != nil {
		if err != nil {
			return nil, "", err
		}
		return m, "old", nil
	}
	return nil, "", nil
}

// readMarkerFile returns (*Marker, error). nil + nil if the path does
// not exist; otherwise reads and unmarshals. Empty / malformed JSON
// returns an error referencing the path that failed.
func readMarkerFile(path string) (*Marker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if m.InstalledChecksums == nil {
		m.InstalledChecksums = make(map[string]string)
	}
	return &m, nil
}

// requireMarker is the strict variant — used by `ndf update` which refuses to
// run outside an ndf project. Matches the bash CLI's error wording.
func requireMarker() *Marker {
	m, err := loadMarker()
	if err != nil {
		die("%v", err)
	}
	if m == nil {
		die("no %s found in current directory. This isn't an ndf project — run `ndf init` first.", projectMarker)
	}
	return m
}

// writeMarker writes the marker atomically (temp + rename) to avoid
// corrupting .ndf.json on a failed write.
//
// Critical correctness rule (preserved from v1.2.2): installedChecksums
// MUST come from the manifest entries we just delivered, not from on-disk
// state of those files. Disk state can drift via customizations; recording
// disk state would cause the next update to silently revert customizations
// (because manifest_sha != installed_sha but current_sha == installed_sha,
// which the diff logic interprets as "framework changed; client unchanged"
// → silent replace). Always pass manifest checksums here.
func writeMarker(m *Marker) error {
	if m.InstalledChecksums == nil {
		m.InstalledChecksums = map[string]string{}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal marker: %w", err)
	}
	data = append(data, '\n')

	// Ensure the marker's parent directory (.ndf/cli/ under v2.5.0+
	// layout) exists before CreateTemp. Covers every caller — cmdInit
	// (fresh project), cmdConfigSet (pre-migration with marker still at
	// OLD: this write creates .ndf/cli/install.json next to the stale
	// OLD copy; the v4.3-to-v4.4 migration's Step 3 cleans up the OLD
	// file), and post-migration cmdUpdate (NEW already exists; MkdirAll
	// is a no-op).
	if err := os.MkdirAll(filepath.Dir(markerPath()), 0o755); err != nil {
		return fmt.Errorf("create marker parent dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(markerPath()), ".ndf.json.*")
	if err != nil {
		return fmt.Errorf("create temp marker: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp marker: %w", err)
	}
	if err := os.Rename(tmpName, markerPath()); err != nil {
		return fmt.Errorf("rename marker: %w", err)
	}
	return nil
}

// stringPtr is a small helper for building *string fields cleanly at call sites.
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// repoSlugRE enforces the OWNER/REPO shape of a GitHub slug. Compiled once
// at init time. Matches the character classes GitHub itself permits:
// letters, digits, period, underscore, hyphen, on each side of the slash.
var repoSlugRE = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// validateRepoSlug enforces OWNER/REPO shape on field-notes repo identifiers.
// Rejects leading/trailing whitespace, multiple slashes, empty halves, and
// any character outside the GitHub-permitted set. Used in three places —
// `ndf init --fieldnotes-repo=...` flag input, the interactive prompt at
// init time, and `ndf config set fieldnotes-repo OWNER/REPO`.
func validateRepoSlug(s string) error {
	if !repoSlugRE.MatchString(s) {
		return fmt.Errorf("expected OWNER/REPO (e.g. nandu-org/Vera-FieldNotes), got %q", s)
	}
	return nil
}
