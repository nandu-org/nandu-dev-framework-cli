package main

import (
	"encoding/json"
	"fmt"
)

// Manifest mirrors the framework repo's manifest.json shape.
//
// Schema is owned by the framework repo (nandu-org/nandu-dev-framework).
// We only consume it. Adding new optional fields here is fine; renaming or
// type-changing existing fields requires a coordinated framework + CLI
// release with min_cli_version bumped.
type Manifest struct {
	Version       string          `json:"version"`
	MinCLIVersion string          `json:"min_cli_version"`
	Files         []ManifestFile  `json:"files"`
	Migrations    []string        `json:"migrations"` // names of pending migration specs (file basenames without .md)
	// Future fields land here; unmarshaling tolerates unknown keys by default.
}

// ManifestFile is one tracked file the framework wants installed in client
// projects.
//
// RenamedFrom signals "this file used to live at <old path>; if the client
// has it there with the previously-installed checksum, move it; if they've
// modified it, surface a diff prompt." The bash CLI walked this with three
// branches in pass 1 of update; we preserve that logic exactly.
type ManifestFile struct {
	Path        string `json:"path"`
	Checksum    string `json:"checksum"`
	RenamedFrom string `json:"renamed_from,omitempty"`
	// UserCustomizable marks a file the framework scaffolds once but the
	// client owns thereafter (e.g. the pre-commit test hook). `update` never
	// silent-replaces these — see handleUserCustomizable in update.go.
	//
	// Optional field; older CLIs that predate it ignore it (JSON tolerates
	// unknown keys), so adding it does NOT force a min_cli_version bump.
	//
	// CORRECTED (framework v4.16.0): the sentence above is true of ADDING the
	// field and false as a general rule, and reading it as general produced a
	// real bug. It was written for framework v4.7.3, which flagged
	// pre-commit-tests.sh WITHOUT changing its content: installedSha ==
	// f.Checksum short-circuits in cmdUpdate before handleUpdate is reached, so
	// an old CLI skips the file and the ignored flag costs nothing. Framework
	// v4.16.0 CHANGED the content of a flagged file. That destroys the
	// short-circuit, routes the file into handleUpdate, and an old CLI — having
	// ignored the flag — treats it as an ordinary tracked file. The premise
	// stopped holding; the conclusion had been carried forward anyway, and
	// v4.16.0 shipped with min_cli_version 2.5.0 until a review caught it.
	//
	// The rule this yields: min_cli_version is not only about MANIFEST FORMAT.
	// It is about any guarantee the manifest makes that only the CLI can keep.
	// An older CLI does not error on this flag — it IGNORES it, which is the
	// most dangerous failure mode available: the manifest promises, the CLI
	// silently declines to deliver, and nothing anywhere reports it. When a
	// release's correctness depends on a CLI behaviour, the floor is that
	// behaviour's version — and RE-DERIVE the floor's argument every time the
	// file it was scoped to changes.
	UserCustomizable bool `json:"user_customizable,omitempty"`
}

// parseManifest decodes a manifest from raw bytes.
func parseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest is not valid JSON: %w", err)
	}
	if m.Version == "" {
		return nil, fmt.Errorf("manifest missing required field: version")
	}
	return &m, nil
}

// pathSet returns the set of paths in this manifest, for fast lookup
// during the "removed files" pass of update.
func (m *Manifest) pathSet() map[string]struct{} {
	s := make(map[string]struct{}, len(m.Files))
	for _, f := range m.Files {
		s[f.Path] = struct{}{}
	}
	return s
}

// renameSources returns the set of `renamed_from` values in this manifest,
// used by the "removed files" pass to skip paths already handled as rename
// sources during pass 1.
func (m *Manifest) renameSources() map[string]struct{} {
	s := make(map[string]struct{})
	for _, f := range m.Files {
		if f.RenamedFrom != "" {
			s[f.RenamedFrom] = struct{}{}
		}
	}
	return s
}
