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
		// The -1 driftCount cases expect the cannot-check line: on a marker
		// whose record predates v2.9.0, silence would read as health (the
		// failure shape the drift hint exists to remove).
		{
			name: "in project, unpinned",
			m:    &Marker{Version: "4.15.0"},
			want: []string{"ndf v9.9.9", "framework v4.15.0", "install record predates CLI v2.9.0 — drift cannot be checked; run `ndf update` once, then `ndf verify`"},
		},
		{
			name: "in project, pinned",
			m:    &Marker{Version: "4.15.0", PinnedVersion: &pin},
			want: []string{"ndf v9.9.9", "framework v4.15.0 (pinned: v4.15.0)", "install record predates CLI v2.9.0 — drift cannot be checked; run `ndf update` once, then `ndf verify`"},
		},
		{
			name: "marker present but no version field",
			m:    &Marker{},
			want: []string{"ndf v9.9.9", "framework (unknown)", "install record predates CLI v2.9.0 — drift cannot be checked; run `ndf update` once, then `ndf verify`"},
		},
		{
			name: "empty pinned string is not annotated",
			m:    &Marker{Version: "4.15.0", PinnedVersion: &empty},
			want: []string{"ndf v9.9.9", "framework v4.15.0", "install record predates CLI v2.9.0 — drift cannot be checked; run `ndf update` once, then `ndf verify`"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := versionLines(cli, c.m, -1)
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

	// Happy path in a project (classifiable record, no drift) — delegates
	// to versionLines, no warning, no extra lines.
	if out, warn := versionOutput(cli, &Marker{Version: "4.15.0"}, nil, 0); warn != "" ||
		!reflect.DeepEqual(out, []string{"ndf v9.9.9", "framework v4.15.0"}) {
		t.Errorf("in-project: out=%v warn=%q; want [ndf v9.9.9, framework v4.15.0] and empty warn", out, warn)
	}

	// Not in a project — CLI line only, no warning.
	if out, warn := versionOutput(cli, nil, nil, -1); warn != "" ||
		!reflect.DeepEqual(out, []string{"ndf v9.9.9"}) {
		t.Errorf("no-project: out=%v warn=%q; want [ndf v9.9.9] and empty warn", out, warn)
	}

	// Corrupt marker — CLI line preserved byte-for-byte, warning surfaced,
	// no framework line, and (by construction) no failure.
	out, warn := versionOutput(cli, nil, errors.New("install.json is not valid JSON"), -1)
	if !reflect.DeepEqual(out, []string{"ndf v9.9.9"}) {
		t.Errorf("corrupt-marker: out=%v; want exactly [ndf v9.9.9] (line 1 must survive)", out)
	}
	if warn == "" {
		t.Error("corrupt-marker: expected a non-empty stderr warning, got none")
	}
}

// TestVersionLinesDriftHint pins the v2.9.0 hint line: a positive drift count
// appends exactly one line pointing at `ndf verify`; zero prints nothing
// extra (healthy projects stay noise-free); -1 (pre-v2.9.0 record) prints
// the cannot-check line — an uncheckable record must say so, because
// silence there reads as health. The hint exists because the framework
// line above it is the install record's CLAIM, and an unchecked claim can
// go stale.
func TestVersionLinesDriftHint(t *testing.T) {
	const cli = "9.9.9"
	m := &Marker{Version: "4.18.0"}

	base := []string{"ndf v9.9.9", "framework v4.18.0"}

	wantCannot := append(append([]string{}, base...), "install record predates CLI v2.9.0 — drift cannot be checked; run `ndf update` once, then `ndf verify`")
	if got := versionLines(cli, m, -1); !reflect.DeepEqual(got, wantCannot) {
		t.Errorf("drift=-1: got %v; want %v (an uncheckable record must say so, not stay silent)", got, wantCannot)
	}
	if got := versionLines(cli, m, 0); !reflect.DeepEqual(got, base) {
		t.Errorf("drift=0: got %v; want %v (clean install must print no hint)", got, base)
	}
	want := append(append([]string{}, base...), "4 file(s) differ from the recorded install — run `ndf verify`")
	if got := versionLines(cli, m, 4); !reflect.DeepEqual(got, want) {
		t.Errorf("drift=4: got %v; want %v", got, want)
	}
}
