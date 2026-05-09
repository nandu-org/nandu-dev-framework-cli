package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// prompt is the basic interactive prompt — prints the question to stderr,
// reads a line from stdin, returns trimmed input. If stdin isn't a TTY
// (e.g., piped, CI), returns the default immediately without blocking.
//
// Stderr is intentional: we want stdout to remain capturable for scripting.
func prompt(question, defaultVal string) string {
	if !isStdinTTY() {
		return defaultVal
	}
	fmt.Fprintf(os.Stderr, "%s ", question)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return defaultVal
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return defaultVal
	}
	return line
}

// promptHidden reads a line of input without echoing it to the terminal —
// used for token entry in `ndf login` so PATs don't leak into terminal
// history or shoulder-surf.
//
// If stdin isn't a TTY, falls back to a normal Read (CI environments
// commonly pipe tokens). The fallback isn't hidden because there's no
// terminal to hide from.
func promptHidden(label string) string {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	if !isStdinTTY() {
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}
	bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // newline after the hidden read
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(bytes), "\r\n")
}

func isStdinTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
