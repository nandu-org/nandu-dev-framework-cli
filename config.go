package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the per-developer config file — tokens, plus a legacy
// fieldnotes_repo field that v1.2.x wrote here. v1.3.0+ stores
// fieldnotes_repo per-project (in .ndf.json), but legacy reads still
// fall back to this file (see resolveFieldnotesRepo).
//
// Schema MUST stay byte-compatible with the bash CLI's writes — deployed
// installs are still reading config files written by ndf v1.x.
type Config struct {
	FrameworkPAT   string `json:"framework_pat,omitempty"`
	FieldnotesPAT  string `json:"fieldnotes_pat,omitempty"`
	FieldnotesRepo string `json:"fieldnotes_repo,omitempty"` // legacy v1.2.x; preserved on rewrite
}

// loadConfig reads the per-developer config. Missing file → empty Config (not
// an error). Malformed file → error with a recovery hint.
func loadConfig() (*Config, error) {
	path := configFile()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON. Inspect it (cat %s) and fix, or rebuild with: rm %s && ndf login (%w)", path, path, path, err)
	}
	return &c, nil
}

// saveConfig writes the per-developer config atomically with mode 0600.
//
// Why atomic write: a half-written config.json would brick subsequent ndf
// invocations until the user manually deleted it. The temp+rename pattern
// guarantees the file either contains the full new content or the previous
// content — never a half-written JSON file.
func saveConfig(c *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		// Best-effort; some Windows filesystems ignore mode bits.
		_ = err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	path := configFile()
	tmp, err := os.CreateTemp(dir, ".config.json.*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		// Same: best-effort on platforms that don't support unix modes.
		_ = err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s → %s: %w", tmpName, path, err)
	}
	return nil
}

// resolveToken returns the framework PAT, env-var-first.
// NDF_GITHUB_TOKEN beats config file; empty if neither.
func resolveToken() string {
	if v := os.Getenv("NDF_GITHUB_TOKEN"); v != "" {
		return v
	}
	c, _ := loadConfig()
	if c == nil {
		return ""
	}
	return c.FrameworkPAT
}

// resolveFieldnotesToken returns the field-notes PAT, env-var-first.
// NDF_FIELDNOTES_TOKEN beats config file; empty if neither.
func resolveFieldnotesToken() string {
	if v := os.Getenv("NDF_FIELDNOTES_TOKEN"); v != "" {
		return v
	}
	c, _ := loadConfig()
	if c == nil {
		return ""
	}
	return c.FieldnotesPAT
}

// resolveFieldnotesRepo returns the field-notes repo for the *current
// directory*. Per-project marker (.ndf.json) takes precedence; legacy
// per-developer config is the fallback. Empty string if neither has it.
func resolveFieldnotesRepo() string {
	if m, err := loadMarker(); err == nil && m != nil && m.FieldnotesRepo != "" {
		return m.FieldnotesRepo
	}
	c, _ := loadConfig()
	if c == nil {
		return ""
	}
	return c.FieldnotesRepo
}

// maskToken renders a PAT for display in `ndf config show` without
// exposing its full value.
func maskToken(t string) string {
	if t == "" {
		return "(not set)"
	}
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}

// markerPath returns the absolute path to the project marker
// (.ndf/cli/install.json under the v2.5.0+ layout). Honors
// CLAUDE_PROJECT_DIR when set (matches Claude Code's convention for
// rooting commands at the project directory); falls back to cwd otherwise.
// Always returns an absolute path so callers can use it from any cwd.
//
// Resolver-vs-write-target: this function stays pure-NEW and returns the
// write target. Read-side dual-path fallback lives in loadMarker (which
// also consults oldMarkerPath when the NEW location is absent).
func markerPath() string {
	return resolveProjectPath(projectMarker)
}

// oldMarkerPath returns the absolute path to the pre-v2.5.0 marker
// location (.ndf.json at the project root). Used by loadMarker's
// dual-path fallback during the catch-up window — i.e., for projects
// whose state lives at the OLD layout until the
// v4.3-to-v4.4-cli-state-relocation migration runs.
func oldMarkerPath() string {
	return resolveProjectPath(oldProjectMarker)
}

// resolveProjectPath returns an absolute path to <rel> under the project
// directory. CLAUDE_PROJECT_DIR (if set) wins; otherwise cwd. Used by
// every path resolver in the CLI so all path helpers share the same
// env-var-aware shape.
func resolveProjectPath(rel string) string {
	projDir := os.Getenv("CLAUDE_PROJECT_DIR")
	if projDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return rel // last-ditch fallback; cwd-relative
		}
		projDir = cwd
	}
	abs, err := filepath.Abs(filepath.Join(projDir, rel))
	if err != nil {
		return filepath.Join(projDir, rel)
	}
	return abs
}

// anchorProjectToCwd makes ndf init / update resolve the project entirely from
// the current working directory, by clearing the $CLAUDE_PROJECT_DIR override for
// the rest of this process.
//
// Why only init/update: these are the commands that write FRAMEWORK FILES
// (fetch/stat/remove/diff/backup, plus init's CLAUDE.project.md) relative to cwd,
// while the marker/sentinels/git resolved via the override — the split-brain
// where the marker records one directory and the files land in another. Clearing
// the override anchors all of it to cwd, so the two can't diverge. (`ndf config
// set` writes no framework files — it only reads and writes the marker, both
// through the same override-aware resolver, so it is already self-consistent and
// is deliberately left honoring $CLAUDE_PROJECT_DIR.)
//
// Why writes differ from reads: the read subcommands (is-project, marker-path,
// config get, config show, version) honor $CLAUDE_PROJECT_DIR because slash
// commands and hooks invoke them from whatever cwd the Bash tool happens to be in
// — they need the override to locate the project. init/update are developer-run
// from the project they intend to change, so after this call a run from outside a
// project simply finds no marker and refuses ("not an ndf project"), rather than
// acting on one directory while writing into another.
//
// Cleared in-process only — each `ndf` invocation is its own process, so this
// never affects a separately-invoked read command.
func anchorProjectToCwd() {
	_ = os.Unsetenv("CLAUDE_PROJECT_DIR")
}

// resolveConfigKey returns (value, source, exists) for a single config key.
// Sources: "marker" or "legacy-config". Exists distinguishes "key is in the
// closed set" (exists=true, possibly with empty value) from "unknown key"
// (exists=false). The closed set is the locked-design contract: callers
// asking for {version, pinned_version, fieldnotes_repo} always get exit 0
// with whatever value is present (empty if no data); any other key is an
// "unknown key" error.
//
// Hyphen→underscore normalization at function entry: callers may pass
// "fieldnotes-repo" or "fieldnotes_repo" interchangeably.
//
// Silent fallback on read errors: a malformed marker or config file yields
// ("", source, true) here rather than failing the whole `config get` call.
// The locked design routes parse-failure reporting through `cmdIsProject`
// (the recommended pre-check); `config get` only reports "unknown key".
func resolveConfigKey(key string) (value, source string, exists bool) {
	// Normalize kebab → snake at entry.
	norm := strings.ReplaceAll(key, "-", "_")
	switch norm {
	case "version":
		m, _ := loadMarker()
		if m == nil {
			return "", "marker", true
		}
		return m.Version, "marker", true
	case "pinned_version":
		m, _ := loadMarker()
		if m == nil || m.PinnedVersion == nil {
			return "", "marker", true
		}
		return *m.PinnedVersion, "marker", true
	case "fieldnotes_repo":
		// Marker first, then legacy config — mirrors resolveFieldnotesRepo().
		if m, _ := loadMarker(); m != nil && m.FieldnotesRepo != "" {
			return m.FieldnotesRepo, "marker", true
		}
		if c, _ := loadConfig(); c != nil && c.FieldnotesRepo != "" {
			return c.FieldnotesRepo, "legacy-config", true
		}
		// Closed-set key, no data anywhere — empty value, canonical source.
		return "", "marker", true
	}
	// Key is NOT in the closed set — caller should exit 2 "unknown key".
	return "", "", false
}

// pendingMigrationPath returns the absolute path to the migration-in-progress
// marker under the v2.5.0+ layout. Honors CLAUDE_PROJECT_DIR (symmetric with
// markerPath). Pure-NEW write target; existence checks should use
// pendingMigrationExists.
func pendingMigrationPath() string {
	return resolveProjectPath(pendingMigrationMarker)
}

// oldPendingMigrationPath returns the absolute path to the pre-v2.5.0
// pending-migration marker location, for dual-path read/cleanup during the
// catch-up window.
func oldPendingMigrationPath() string {
	return resolveProjectPath(oldPendingMigrationMarker)
}

// pendingHandoffPath returns the absolute path to the migration
// team-handoff marker under the v2.5.0+ layout. Honors
// CLAUDE_PROJECT_DIR (symmetric with markerPath). Pure-NEW write target;
// existence/load should use pendingHandoffExists / loadPendingHandoff.
func pendingHandoffPath() string {
	return resolveProjectPath(pendingHandoffMarker)
}

// oldPendingHandoffPath returns the absolute path to the pre-v2.5.0
// pending-handoff marker location, for dual-path read/cleanup during the
// catch-up window.
func oldPendingHandoffPath() string {
	return resolveProjectPath(oldPendingHandoffMarker)
}

// migrationSentinelPath returns the absolute path to a single migration's
// sentinel under the v2.5.0+ layout. Pure-NEW write target — new
// sentinels (written by /ndf-migrate post-v4.4.0) always land here.
// Existence checks during the catch-up window should use
// migrationSentinelExists, which consults both NEW and OLD paths.
func migrationSentinelPath(name string) string {
	return resolveProjectPath(filepath.Join(migrationsSentinelDir, name+".complete"))
}

// oldMigrationSentinelPath returns the absolute path to a single
// migration's sentinel at the pre-v2.5.0 location. Read-only — used by
// migrationSentinelExists for the dual-path existence check during the
// catch-up window.
func oldMigrationSentinelPath(name string) string {
	return resolveProjectPath(filepath.Join(oldMigrationsSentinelDir, name+".complete"))
}

// migrationSentinelExists returns true if a sentinel for `name` is on
// disk at EITHER the new (.ndf/cli/sentinels/) or old (.ndf-migrations/)
// location. Sentinels are append-only: once written by /ndf-migrate they
// are never moved by anything except the v4.3-to-v4.4 migration. The
// dual-path check makes the catch-up window transparent — a stale client
// running `ndf update` against framework v4.4.0 still has its prior
// migration sentinels at OLD until the relocation migration runs.
func migrationSentinelExists(name string) bool {
	if fileExists(migrationSentinelPath(name)) {
		return true
	}
	return fileExists(oldMigrationSentinelPath(name))
}

// pendingMigrationExists returns true if a pending-migration marker is on
// disk at EITHER NEW or OLD path. Used by consumePendingHandoff's
// defense-in-depth guard (which bails out if the marker is still present,
// signaling that /ndf-migrate didn't finish cleanly).
func pendingMigrationExists() bool {
	if fileExists(pendingMigrationPath()) {
		return true
	}
	return fileExists(oldPendingMigrationPath())
}

// pendingHandoffExists returns true if a pending-handoff marker is on
// disk at EITHER NEW or OLD path. Analogous shape to
// pendingMigrationExists; reserved for future call sites that need a
// quick "is there a pending handoff?" check without loading the file.
func pendingHandoffExists() bool {
	if fileExists(pendingHandoffPath()) {
		return true
	}
	return fileExists(oldPendingHandoffPath())
}

// fileExists is a small helper: true iff os.Stat succeeds on `path`.
// Used by the dual-path *Exists helpers and by cmdInit's existence guard.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
