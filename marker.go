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
//	  "fieldnotes_repo":     "owner/repo",  // optional
//	  "user_customizable":   [ "<path>", ... ],  // v2.9.0+ (null = legacy record)
//	  "pending_reoffer":     [ "<path>", ... ]   // optional, v2.9.0+
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
	// UserCustomizable lists the manifest paths flagged user_customizable at
	// the last init/update, so offline commands (`ndf verify`, `ndf version`)
	// can tell "customized by the client (by design)" apart from real drift
	// without a network fetch. Additive since v2.9.0. Deliberately NOT
	// omitempty: nil (field absent — the record predates v2.9.0, or an older
	// CLI's writeMarker stripped it on its last update) must stay
	// distinguishable from an empty non-nil list (a manifest that genuinely
	// flags nothing). On nil, verify/version degrade to "unclassified"
	// rather than guessing; on empty they classify normally.
	UserCustomizable []string `json:"user_customizable"`
	// PendingReoffer lists paths whose next `ndf update` must prompt, even
	// when the install record says they are current. Three writers: `ndf
	// verify --fix`; any skip or failed action that was not a typed human
	// decision (an unattended conflict/rename auto-skip on a recorded path
	// gains a mark it never had — deliberate: verify then reports "awaiting
	// decision", the truth about a scheduled prompt); and any declined
	// net-new collision, typed or not (an entry-less path has no ledger
	// entry, so the mark is its only visibility). Kept OUT of
	// installed_checksums on purpose — the re-offer is an intent, and the
	// entry must keep recording the last actually-delivered version (pass
	// 2's removal guard and the rename fast-path depend on it). The
	// clearing policy lives in ONE place — keepPendingMark (update.go):
	// any delivery clears; a TYPED skip on a recorded path clears; an
	// unattended auto-skip or a failed [r]/[b] action keeps (not
	// decisions); an entry-less decline keeps (the mark is its only
	// visibility). A path that has become user_customizable or left the
	// manifest clears — moot. Additive; an older CLI strips it on write
	// (documented in CHANGELOG).
	PendingReoffer []string `json:"pending_reoffer,omitempty"`
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
// Critical correctness rule, two halves — both are load-bearing:
//
//  1. (preserved from v1.2.2) installedChecksums must NEVER record on-disk
//     state. Disk state can drift via customizations; recording it would make
//     the next update read "framework changed; client unchanged"
//     (manifest_sha != installed_sha, current_sha == installed_sha) and
//     silently revert the customization via the silent-replace branch.
//
//  2. (since v2.9.0) installedChecksums must record only what was actually
//     DELIVERED — the literal reading of rule 1's "entries we just
//     delivered". Recording the whole target manifest regardless of per-file
//     outcome made the ledger assert installations that never happened: a
//     file skipped at a prompt was recorded as current, every later update
//     short-circuited on manifest_sha == installed_sha without looking at
//     it, and the project ran files several releases old under a current
//     version number — invisibly, indefinitely (a 2026-07 field diagnosis
//     found a client project running four framework files seven releases
//     behind their recorded version). A skipped file therefore carries its
//     PREVIOUS installed checksum forward (the last version actually
//     delivered), so the divergence stays visible and the next update
//     re-offers it. The same carry-forward applies to a preserved
//     user_customizable file the update chose not to touch — its entry
//     keeps the last delivered placeholder reference, not the new
//     manifest's. See ledgerAfterUpdate in update.go.
//
// Both halves reduce to: the ledger records the framework's last delivery,
// never disk state and never mere intent. Re-offer intent lives in the
// separate PendingReoffer field, never in the checksums — an earlier
// design marked entries with an in-band sentinel and was rejected in
// review: it destroyed the last-delivered value (which the pass-2 removal
// guard and the rename fast-path compare against) and, because the marker
// is committed and team-shared, turned one developer's local intent into
// permanent phantom prompts on every teammate's pristine checkout.
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
	// file), cmdVerify's --fix (same pre-migration shape as cmdConfigSet
	// in principle, though unreachable in practice: an OLD-only marker is
	// by construction pre-v2.9.0, so verify classifies everything
	// unclassified and --fix finds nothing to mark), and post-migration
	// cmdUpdate (NEW already exists; MkdirAll is a no-op).
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
		return fmt.Errorf("expected OWNER/REPO (e.g. nandu-org/Example-FieldNotes), got %q", s)
	}
	return nil
}
