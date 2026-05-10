package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// Marker is the per-project pinning + checksum file (.ndf.json).
//
// Schema MUST stay byte-compatible with what the bash CLI v1.x writes —
// every deployed canary (Vera) and any client install on disk has v1.x's
// JSON layout, and `ndf update` from this v2.x binary needs to read it
// faithfully on the first run. After the first v2.x update, we re-emit
// the same shape.
//
//	{
//	  "version":             "3.3.3",
//	  "pinned_version":      null | "3.3.3",
//	  "installed_checksums": { "<path>": "<sha256>", ... },
//	  "fieldnotes_repo":     "owner/repo"   // optional
//	}
type Marker struct {
	Version            string            `json:"version"`
	PinnedVersion      *string           `json:"pinned_version"` // null when not pinned
	InstalledChecksums map[string]string `json:"installed_checksums"`
	FieldnotesRepo     string            `json:"fieldnotes_repo,omitempty"`
}

// loadMarker reads .ndf.json from cwd. nil + nil if absent.
// Empty file returns an error (treat as malformed).
func loadMarker() (*Marker, error) {
	data, err := os.ReadFile(markerPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", markerPath(), err)
	}
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", markerPath(), err)
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

	tmp, err := os.CreateTemp(".", ".ndf.json.*")
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
