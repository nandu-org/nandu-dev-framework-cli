package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// sha256File returns the lowercase hex sha256 of the file's bytes.
// Format matches `sha256sum file | awk '{print $1}'` exactly so checksums
// emitted by this binary are identical to those v1.x bash CLI emitted.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sha256OfFileOrEmpty is the version that simply returns "" for missing files.
// The update flow uses this in several "did the client modify it?" branches
// where a missing file is a real distinct case from a different-content file.
func sha256OfFileOrEmpty(path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	s, err := sha256File(path)
	if err != nil {
		return ""
	}
	return s
}
