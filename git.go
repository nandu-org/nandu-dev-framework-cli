package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// gitInRepo runs `git -C <dir> <args...>` and returns trimmed stdout.
// Non-zero exit returns an error including stderr so the user sees git's
// actual diagnostic.
func gitInRepo(dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// gitInRepoStreaming runs git with stdout/stderr forwarded to the user's
// terminal — used for `git push` so the user sees progress and any
// password/2FA prompts that ssh-agent or git-credential-manager surface.
func gitInRepoStreaming(dir string, args ...string) error {
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	cmd.Stdout = os.Stderr // ndf's convention: progress on stderr
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin // for credential prompts
	return cmd.Run()
}

// gitIsRepo returns true if dir is inside a git working tree.
func gitIsRepo(dir string) bool {
	_, err := gitInRepo(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// gitCurrentBranch returns the name of the current branch (empty in detached HEAD).
func gitCurrentBranch(dir string) string {
	out, err := gitInRepo(dir, "branch", "--show-current")
	if err != nil {
		return ""
	}
	return out
}

// gitHasUncommittedChanges returns true if there are unstaged, staged, or
// untracked changes in the working tree.
//
// Uses `git status --porcelain` (broader than the bash CLI's `git diff
// --quiet`, which missed untracked files). For our use case — committing
// framework updates — untracked files like newly-introduced agent specs
// SHOULD count as "uncommitted changes coworkers need to see."
func gitHasUncommittedChanges(dir string) bool {
	out, err := gitInRepo(dir, "status", "--porcelain")
	if err != nil {
		return false
	}
	return out != ""
}
