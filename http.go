package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// HTTP timeout for any single GitHub fetch. The framework files are tiny
// (few KB each) so a generous timeout protects against transient blips
// without making a wedged request hold up the user.
const httpTimeout = 30 * time.Second

// httpClient is the package-wide client. Keep one instance so connection
// reuse works across multiple file fetches in `ndf update`.
var httpClient = &http.Client{Timeout: httpTimeout}

// authedGET performs an authenticated GET against GitHub's API or
// raw.githubusercontent.com. Returns the response body or an error
// describing the HTTP status; non-2xx responses are errors.
func authedGET(url string) ([]byte, error) {
	pat := resolveToken()
	if pat == "" {
		return nil, fmt.Errorf("no GitHub PAT configured. Run `ndf login` first")
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("User-Agent", "ndf-cli/"+CLIVersion)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body of %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Surface body in the error — GitHub's 401/404 bodies are usually
		// useful ("Bad credentials" / "Not Found").
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, string(body))
	}
	return body, nil
}

// fetchManifest pulls the manifest.json at the given ref (a tag like "v3.3.3"
// or a branch like "main").
func fetchManifest(ref string) (*Manifest, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/manifest.json", FrameworkRepo, ref)
	body, err := authedGET(url)
	if err != nil {
		return nil, err
	}
	return parseManifest(body)
}

// fetchFileTo writes a file from the framework repo to a destination on disk.
// Creates parent directories. Atomic via temp+rename so a partial download
// can't corrupt an existing file.
func fetchFileTo(ref, repoPath, dest string) error {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", FrameworkRepo, ref, repoPath)
	body, err := authedGET(url)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create %s parent: %w", dest, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".ndf-fetch.*")
	if err != nil {
		return fmt.Errorf("create temp %s: %w", dest, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("rename → %s: %w", dest, err)
	}
	return nil
}

// listSemverTags hits the GitHub tags API and returns all v?D.D.D tag names
// found, in API order. Caller picks the latest with pickLatestSemverTag.
func listSemverTags() ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/tags?per_page=100", FrameworkRepo)
	body, err := authedGET(url)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse tags response: %w", err)
	}
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		if parseSemver(t.Name) != nil {
			out = append(out, t.Name)
		}
	}
	return out, nil
}

// resolveRef converts a user-facing version request into a git ref.
//
//   - empty           → resolve "latest" via the tags API. If no semver tags
//                       exist (brand-new repo, etc), warn and fall back to "main".
//   - "v3.3.3" or "3.3.3" → normalize to "v3.3.3"
//
// Mirrors the bash CLI's `_resolve_ref` exactly so the same inputs map to
// the same refs.
func resolveRef(version string) (string, error) {
	if version == "" {
		tags, err := listSemverTags()
		if err != nil {
			return "", err
		}
		latest := pickLatestSemverTag(tags)
		if latest == "" {
			warn("no version tags found in repo; falling back to `main`")
			return "main", nil
		}
		return latest, nil
	}
	v := version
	if len(v) > 0 && v[0] == 'v' {
		v = v[1:]
	}
	return "v" + v, nil
}

// checkMinCLIVersion aborts with a useful upgrade message if the manifest
// requires a newer CLI than this binary.
func checkMinCLIVersion(m *Manifest) {
	min := m.MinCLIVersion
	if min == "" {
		return
	}
	if semverLess(CLIVersion, min) {
		die("your CLI (v%s) is older than this release requires (min v%s). Re-install: re-run the install one-liner from the onboarding email.", CLIVersion, min)
	}
}
