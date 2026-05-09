package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// printDiff writes a unified-style diff between two byte slices to stderr.
// The bash CLI shelled out to `diff -u`; we generate the same shape inline
// so we don't need diff(1) on Windows. Output is purely informational —
// it helps the user decide [r]eplace / [s]kip / [b]ackup at the prompt.
//
// This is not a true LCS-based unified diff — it's a "show both versions
// inline with markers" presentation tuned for the conflict case where the
// user just needs to see "what's the framework saying vs. what I have."
// For tiny files (the framework files are 30-300 lines), it's perfectly
// readable; for huge files, the user can `diff -u` themselves separately.
//
// Callers pass labels ("yours", "framework") so it's clear which side is which.
func printDiff(aLabel string, a []byte, bLabel string, b []byte) {
	if bytes.Equal(a, b) {
		return
	}
	w := os.Stderr

	fmt.Fprintf(w, "  --- %s\n", aLabel)
	fmt.Fprintf(w, "  +++ %s\n", bLabel)

	aLines := splitLines(a)
	bLines := splitLines(b)

	// Lightweight common-prefix / common-suffix trim, then everything in the
	// middle prints as "- yours / + framework" blocks. For framework file
	// review use this is plenty — the user wants to see what differs, not
	// rigorously locate every minimal hunk.
	prefix := commonPrefix(aLines, bLines)
	suffix := commonSuffix(aLines[prefix:], bLines[prefix:])

	if prefix > 0 {
		fmt.Fprintf(w, "  @@ context: first %d line(s) match @@\n", prefix)
	}

	for _, l := range aLines[prefix : len(aLines)-suffix] {
		fmt.Fprintf(w, "  - %s\n", l)
	}
	for _, l := range bLines[prefix : len(bLines)-suffix] {
		fmt.Fprintf(w, "  + %s\n", l)
	}

	if suffix > 0 {
		fmt.Fprintf(w, "  @@ context: last %d line(s) match @@\n", suffix)
	}
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	var out []string
	s := bufio.NewScanner(bytes.NewReader(b))
	s.Buffer(make([]byte, 1024*1024), 8*1024*1024) // tolerate long lines
	for s.Scan() {
		out = append(out, s.Text())
	}
	if !strings.HasSuffix(string(b), "\n") && len(out) > 0 {
		// already captured; nothing to do
	}
	return out
}

func commonPrefix(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func commonSuffix(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}
	return n
}

// readAll is a tiny helper for slurping a small file.
func readAll(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
