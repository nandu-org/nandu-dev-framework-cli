package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAnchorProjectToCwd verifies that ndf init / update drop the
// $CLAUDE_PROJECT_DIR override so project paths resolve from cwd. This is the
// guarantee that keeps them from splitting the marker and the framework files
// across two directories. (ndf config set is deliberately NOT anchored — it
// writes no framework files, so it cannot split-brain.)
func TestAnchorProjectToCwd(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "/some/other/project")

	// Precondition: with the override set, resolution honors it (read-style).
	if got := resolveProjectPath(projectMarker); !strings.HasPrefix(got, "/some/other/project/") {
		t.Fatalf("precondition failed: expected the override to be honored, got %q", got)
	}

	anchorProjectToCwd()

	// After anchoring: the override is cleared and resolution falls back to cwd.
	if v, ok := os.LookupEnv("CLAUDE_PROJECT_DIR"); ok {
		t.Fatalf("CLAUDE_PROJECT_DIR still set after anchorProjectToCwd: %q", v)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cwd, projectMarker)
	if got := resolveProjectPath(projectMarker); got != want {
		t.Errorf("after anchorProjectToCwd: resolveProjectPath(%q) = %q; want %q (cwd-relative)", projectMarker, got, want)
	}
}
