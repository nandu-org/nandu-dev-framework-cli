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
// Schema MUST stay byte-compatible with the bash CLI's writes — Vera and
// any other deployed install is reading config files written by ndf v1.x.
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

// markerPath returns the absolute path to the project marker (.ndf.json).
// Honors CLAUDE_PROJECT_DIR when set (matches Claude Code's convention for
// rooting commands at the project directory); falls back to cwd otherwise.
// Always returns an absolute path so callers can use it from any cwd.
func markerPath() string {
	projDir := os.Getenv("CLAUDE_PROJECT_DIR")
	if projDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return projectMarker // last-ditch fallback; cwd-relative
		}
		projDir = cwd
	}
	abs, err := filepath.Abs(filepath.Join(projDir, projectMarker))
	if err != nil {
		return filepath.Join(projDir, projectMarker)
	}
	return abs
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

// pendingMigrationPath returns the path to the migration-in-progress marker.
func pendingMigrationPath() string {
	return pendingMigrationMarker
}

// pendingHandoffPath returns the path to the migration team-handoff marker.
func pendingHandoffPath() string {
	return pendingHandoffMarker
}

// migrationSentinelPath returns the path to a single migration's sentinel.
func migrationSentinelPath(name string) string {
	return filepath.Join(migrationsSentinelDir, name+".complete")
}
