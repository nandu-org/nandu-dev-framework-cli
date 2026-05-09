package main

import (
	"fmt"
	"os"
)

// Output helpers — match the bash CLI's prefixing exactly so users moving from
// v1.x to v2.x don't notice the implementation switch in their terminal.
//
// Convention (preserved from bash):
//   _die  → stderr, "ndf: error: ..." + exit 1
//   _warn → stderr, "ndf: warn: ..."
//   _info → stderr, "ndf: ..."           (non-error progress)
//   _ok   → stdout, "ndf: ..."           (success / final result)
//
// Stderr vs stdout matters for scripting: success output (paths, versions)
// goes to stdout so it can be captured; progress + warnings go to stderr.

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ndf: error: "+format+"\n", args...)
	os.Exit(1)
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ndf: warn: "+format+"\n", args...)
}

func info(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ndf: "+format+"\n", args...)
}

func ok(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "ndf: "+format+"\n", args...)
}

// raw* variants print without the "ndf:" prefix — used for the team handoff
// block (which is meant to be pasted as-is into team chat) and for the
// embedded help text.
func rawOut(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

func rawErr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
