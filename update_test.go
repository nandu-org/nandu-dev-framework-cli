package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCustomizableAction pins the marker-independent decision table for
// user_customizable files (the #11 safeguard).
func TestCustomizableAction(t *testing.T) {
	cases := []struct{ current, manifest, want string }{
		{"", "abc", "create"},          // absent on disk -> deliver placeholder
		{"abc", "abc", "skip"},         // disk == framework -> nothing to do
		{"deadbeef", "abc", "preserve"}, // disk differs (client-owned) -> never overwrite
	}
	for _, c := range cases {
		if got := customizableAction(c.current, c.manifest); got != c.want {
			t.Errorf("customizableAction(%q,%q)=%q want %q", c.current, c.manifest, got, c.want)
		}
	}
}

// TestHandleUserCustomizablePreservesCustomization is the synthetic reproduction
// of the field-note bug (ndf update overwriting a customized hook script). It
// asserts the safeguard preserves a client-customized file and is a no-op when
// the file already matches the framework — exercising the no-network paths.
// Crucially, no marker is consulted at all, so the protection holds even when
// the marker's installed_checksums entry is missing or stale.
func TestHandleUserCustomizablePreservesCustomization(t *testing.T) {
	dir := t.TempDir()
	// f.Path is project-relative; cmdUpdate runs from the project root. Chdir
	// (manual + defer, not t.Chdir, to stay compatible with the module's
	// declared go1.22).
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	rel := ".claude/hooks/pre-commit-tests.sh"
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := []byte("#!/usr/bin/env bash\nmake test\n")
	if err := os.WriteFile(rel, custom, 0o755); err != nil {
		t.Fatal(err)
	}

	// (1) Manifest checksum differs from the client's file -> preserve, untouched.
	f := ManifestFile{Path: rel, Checksum: "placeholder_sha_that_differs", UserCustomizable: true}
	var changes []change
	var preserved []string
	handleUserCustomizable("unused-ref", f, &changes, &preserved)

	if got, _ := os.ReadFile(rel); string(got) != string(custom) {
		t.Fatalf("customized file was modified: got %q", got)
	}
	if len(preserved) != 1 || preserved[0] != rel {
		t.Fatalf("expected %s preserved, got %v", rel, preserved)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes recorded, got %v", changes)
	}

	// (2) File already equals the framework version -> skip (no-op).
	f2 := ManifestFile{Path: rel, Checksum: sha256OfFileOrEmpty(rel), UserCustomizable: true}
	var changes2 []change
	var preserved2 []string
	handleUserCustomizable("unused-ref", f2, &changes2, &preserved2)
	if len(preserved2) != 0 || len(changes2) != 0 {
		t.Fatalf("matching file should be a no-op; preserved=%v changes=%v", preserved2, changes2)
	}
}
