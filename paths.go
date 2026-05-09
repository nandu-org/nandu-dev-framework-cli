package main

import (
	"os"
	"path/filepath"
)

// Per-developer config (tokens) — same path on macOS and Linux as the bash
// CLI used. On Windows we honor XDG_CONFIG_HOME if set, else fall back to
// %APPDATA%\nandu\config.json which is the Windows-native equivalent.
//
// We deliberately do NOT use a different per-OS shape — the file content is
// the same JSON schema everywhere, just the host directory differs.

const (
	// projectMarker is the per-project pinning + checksum file. Lives in
	// the project root. Always this exact filename.
	projectMarker = ".ndf.json"

	// pendingMigrationMarker is written by `ndf update` when a manifest
	// declares one or more migrations that haven't been applied yet, so
	// /ndf-migrate knows which specs to run.
	pendingMigrationMarker = ".ndf-pending-migration"

	// migrationsSentinelDir holds <name>.complete files written by
	// /ndf-migrate after a migration spec runs successfully.
	migrationsSentinelDir = ".ndf-migrations"
)

// configDir returns the per-developer config directory.
//   - $XDG_CONFIG_HOME/nandu  (if set)
//   - ~/.config/nandu          (Unix default)
//   - %APPDATA%\nandu          (Windows default — os.UserConfigDir handles this)
func configDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "nandu")
	}
	d, err := os.UserConfigDir()
	if err != nil {
		// Fallback to ~/.config/nandu — covers any pathological env.
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "nandu")
	}
	return filepath.Join(d, "nandu")
}

func configFile() string {
	return filepath.Join(configDir(), "config.json")
}
