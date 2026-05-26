package main

import (
	"fmt"
	"os"
)

// cmdIsProject — does the cwd (or $CLAUDE_PROJECT_DIR) contain a parseable .ndf.json?
// Exit codes:
//
//	0 — yes (marker exists and parses)
//	1 — no (marker absent)
//	2 — internal error (marker exists but unreadable/malformed; stderr message + ndf:internal-error stdout marker as fallback for environments that swallow stderr)
//
// Silent on 0 and 1 — caller decides what to print.
func cmdIsProject(args []string) {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			rawErr("Usage: ndf is-project\n\nExit 0 if cwd (or $CLAUDE_PROJECT_DIR) is an NDF project; 1 if not; 2 on internal error.")
			return
		}
	}
	m, err := loadMarker()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Println("ndf:internal-error")
		os.Exit(2)
	}
	if m == nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// cmdMarkerPath — print the absolute marker path the CLI would consult.
// Exit codes: 0 — printed; 2 — internal error.
// Does NOT require the marker to exist (returns the resolved path either way);
// callers can pair with `ndf is-project` if they need existence.
func cmdMarkerPath(args []string) {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			rawErr("Usage: ndf marker-path\n\nPrint the absolute path to .ndf.json the CLI would consult (honors $CLAUDE_PROJECT_DIR). Does not check existence.")
			return
		}
	}
	fmt.Println(markerPath())
}
