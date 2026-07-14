package main

import (
	"errors"
	"reflect"
	"testing"
)

// TestVersionLines pins the `ndf version` output shape: line 1 is always the
// CLI version (byte-stable for consumers), and a framework line is appended
// only when a marker is present — with a pinned annotation when pinned_version
// is set. A stable synthetic CLI version keeps the assertions independent of
// the real CLIVersion constant.
func TestVersionLines(t *testing.T) {
	const cli = "9.9.9"
	pin := "4.15.0"
	empty := ""

	cases := []struct {
		name string
		m    *Marker
		want []string
	}{
		{
			name: "not in a project — CLI line only",
			m:    nil,
			want: []string{"ndf v9.9.9"},
		},
		{
			name: "in project, unpinned",
			m:    &Marker{Version: "4.15.0"},
			want: []string{"ndf v9.9.9", "framework v4.15.0"},
		},
		{
			name: "in project, pinned",
			m:    &Marker{Version: "4.15.0", PinnedVersion: &pin},
			want: []string{"ndf v9.9.9", "framework v4.15.0 (pinned: v4.15.0)"},
		},
		{
			name: "marker present but no version field",
			m:    &Marker{},
			want: []string{"ndf v9.9.9", "framework (unknown)"},
		},
		{
			name: "empty pinned string is not annotated",
			m:    &Marker{Version: "4.15.0", PinnedVersion: &empty},
			want: []string{"ndf v9.9.9", "framework v4.15.0"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := versionLines(cli, c.m)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("versionLines(%q, %+v) = %v; want %v", cli, c.m, got, c.want)
			}
		})
	}
}

// TestVersionOutput pins the marker-load → output mapping, including the
// headline guarantee: a corrupt marker (loadErr != nil) still yields the CLI
// line (byte-identical to the happy path) plus a non-empty stderr warning, and
// never signals failure. Covers cmdVersion's degradation branch without a
// process spawn.
func TestVersionOutput(t *testing.T) {
	const cli = "9.9.9"

	// Happy path in a project — delegates to versionLines, no warning.
	if out, warn := versionOutput(cli, &Marker{Version: "4.15.0"}, nil); warn != "" ||
		!reflect.DeepEqual(out, []string{"ndf v9.9.9", "framework v4.15.0"}) {
		t.Errorf("in-project: out=%v warn=%q; want [ndf v9.9.9, framework v4.15.0] and empty warn", out, warn)
	}

	// Not in a project — CLI line only, no warning.
	if out, warn := versionOutput(cli, nil, nil); warn != "" ||
		!reflect.DeepEqual(out, []string{"ndf v9.9.9"}) {
		t.Errorf("no-project: out=%v warn=%q; want [ndf v9.9.9] and empty warn", out, warn)
	}

	// Corrupt marker — CLI line preserved byte-for-byte, warning surfaced,
	// no framework line, and (by construction) no failure.
	out, warn := versionOutput(cli, nil, errors.New("install.json is not valid JSON"))
	if !reflect.DeepEqual(out, []string{"ndf v9.9.9"}) {
		t.Errorf("corrupt-marker: out=%v; want exactly [ndf v9.9.9] (line 1 must survive)", out)
	}
	if warn == "" {
		t.Error("corrupt-marker: expected a non-empty stderr warning, got none")
	}
}
