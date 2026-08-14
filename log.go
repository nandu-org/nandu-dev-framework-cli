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
//
// Each helper renders the caller's format via fmt.Sprintf and prints the
// result with a fixed "%s" format. NOT an optimization — a correctness
// guard: the previous shape ("ndf: warn: "+format) built the format string
// by concatenation, which defeats go vet's printf inference entirely, and
// this repo has already shipped a mangled-help-text bug (%!A(MISSING)) that
// vet would have caught. With Sprintf(format, args...) as a direct call,
// vet checks every call site's verbs against its arguments again.

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ndf: error: %s\n", fmt.Sprintf(format, args...))
	os.Exit(1)
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ndf: warn: %s\n", fmt.Sprintf(format, args...))
}

func info(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ndf: %s\n", fmt.Sprintf(format, args...))
}

func ok(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "ndf: %s\n", fmt.Sprintf(format, args...))
}

// raw* variants print without the "ndf:" prefix — used for the team handoff
// block (which is meant to be pasted as-is into team chat) and for the
// embedded help text.
func rawOut(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "%s\n", fmt.Sprintf(format, args...))
}

func rawErr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s\n", fmt.Sprintf(format, args...))
}
