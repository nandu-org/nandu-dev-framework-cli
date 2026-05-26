package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// Per-developer config (tokens) — same path on macOS and Linux as the bash
// CLI used. On Windows we honor XDG_CONFIG_HOME if set, else fall back to
// %APPDATA%\nandu\config.json which is the Windows-native equivalent.
//
// We deliberately do NOT use a different per-OS shape — the file content is
// the same JSON schema everywhere, just the host directory differs.

const (
	// projectMarker is the per-project pinning + checksum file. Lives at
	// .ndf/cli/install.json relative to the project root (v2.5.0+ layout —
	// CLI-managed state consolidated under .ndf/cli/ in Release 3).
	projectMarker = ".ndf/cli/install.json"

	// pendingMigrationMarker is written by `ndf update` when a manifest
	// declares one or more migrations that haven't been applied yet, so
	// /ndf-migrate knows which specs to run.
	pendingMigrationMarker = ".ndf/cli/pending-migration"

	// pendingHandoffMarker is written by `ndf update` on the gate-fired
	// run (when a migration spec is being pre-delivered) and consumed on
	// the next `ndf update` run (after /ndf-migrate has executed the
	// spec). It carries migration-specific team-handoff text that the
	// migrator pastes into team chat alongside the standard handoff.
	pendingHandoffMarker = ".ndf/cli/pending-handoff"

	// migrationsSentinelDir holds <name>.complete files written by
	// /ndf-migrate after a migration spec runs successfully.
	migrationsSentinelDir = ".ndf/cli/sentinels"

	// Old (pre-v2.5.0) path constants — referenced by the dual-path read
	// fallback during the catch-up window. Clients on pre-v4.4.0
	// frameworks have their state at these paths until the
	// v4.3-to-v4.4-cli-state-relocation migration moves it. The CLI reads
	// from either location (NEW first, OLD fallback); writes always land
	// at NEW. These constants can be retired once roster-wide
	// post-migration confirmation is in place (see CHANGELOG and KB
	// §Beyond v4.4.0).
	oldProjectMarker          = ".ndf.json"
	oldPendingMigrationMarker = ".ndf-pending-migration"
	oldPendingHandoffMarker   = ".ndf-pending-handoff"
	oldMigrationsSentinelDir  = ".ndf-migrations"
)

// configDir returns the per-developer config directory.
//   - $XDG_CONFIG_HOME/nandu  (if set)
//   - ~/.config/nandu          (macOS + Linux — matches bash CLI v1.x writes)
//   - %APPDATA%\nandu          (Windows — os.UserConfigDir handles this)
//
// On macOS, os.UserConfigDir returns ~/Library/Application Support, which
// would NOT match the bash CLI's ~/.config/nandu path. Byte-compatibility
// with v1.x-written config files requires we hardcode ~/.config on macOS.
func configDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "nandu")
	}
	if runtime.GOOS == "windows" {
		if d, err := os.UserConfigDir(); err == nil {
			return filepath.Join(d, "nandu")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nandu")
}

func configFile() string {
	return filepath.Join(configDir(), "config.json")
}
