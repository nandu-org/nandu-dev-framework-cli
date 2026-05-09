# Releasing a new `ndf` CLI version

Mechanical runbook for shipping an `ndf` CLI release. End-to-end: from version bump to all distribution channels updated. Audience is whoever holds write access to `nandu-org/nandu-dev-framework-cli`, the Homebrew tap, and the Scoop bucket.

## What gets shipped

A single CLI release produces:

- **Four binaries** for `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `windows/amd64`, attached to a GitHub Release at `https://github.com/nandu-org/nandu-dev-framework-cli/releases/tag/v<X.Y.Z>`, alongside a `checksums.txt` with sha256 per file.
- **A bumped Homebrew formula** at `nandu-org/homebrew-tap` (`Formula/ndf.rb`).
- **A bumped Scoop manifest** at `nandu-org/scoop-bucket` (`bucket/ndf.json`).
- **A CHANGELOG entry** in this repo (`CHANGELOG.md`).

The release pipeline (matrix build + GitHub Release upload) is automated by `.github/workflows/release.yml`, fired by a `v*.*.*` tag push. The tap and bucket bumps are still manual (one PR each).

## Versioning policy

Semver applies to the CLI binary only. The framework files have their own version (see `ndf-maintainer` skill).

| Bump | When |
|---|---|
| **Patch** (`vX.Y.Z+1`) | Bug fix, error-message improvement, doc fix, no behavior change |
| **Minor** (`vX.Y+1.0`) | New flag, new subcommand, new capability; backwards-compatible |
| **Major** (`vX+1.0.0`) | Breaking change to flags / config schema / on-disk formats, or implementation rewrite (e.g. v1 → v2 bash → Go) |

When in doubt: bump conservatively (more patches, fewer minors). Anything that changes how a deployed install reads its existing files (config, marker, sentinel) is at minimum a minor bump and probably needs a migration plan.

## Pre-flight checks

Before tagging:

1. **All tests pass.** `go test ./...` (when tests exist; v2.0.x has none yet — adding them is on the roadmap).
2. **`go vet ./...` clean.**
3. **Working tree clean** on `main`. No uncommitted state.
4. **Local build runs natively on macOS.** GitHub Actions cross-compiles, but the maintainer's Mac is the first place a darwin binary actually executes:
   ```bash
   go build -ldflags="-s -w -X main.CLIVersion=<next-version>" -o /tmp/ndf-local .
   /tmp/ndf-local version
   /tmp/ndf-local help
   ```
5. **Smoke test against an existing project.** Use whichever NDF-tracked project you have locally (the canary, your own machine):
   ```bash
   cd <some-ndf-project>
   /tmp/ndf-local update
   ```
   This catches regressions in config-file reads, manifest parsing, and the update flow. Skipping this check is how the v2.0.0 macOS config-path bug got out the door — `version` and `help` don't load config, so they don't surface that class of bug.
6. **Read the CHANGELOG draft once cold.** No internal commentary, no project-specific names, no rationale paragraphs. Sanitized factual notes only.

If any check fails, fix it before tagging.

## Release pipeline

### 1. Bump version + write CHANGELOG

```bash
cd nandu-dev-framework-cli
```

Edit `version.go`:
```go
var CLIVersion = "<next-version>"
```

Add a CHANGELOG entry at the top of `CHANGELOG.md`:
```markdown
## v<next-version> — <YYYY-MM-DD>

**One-line summary.**

### What's new / Fixed
- Bullet per change. Sanitized — what changed and what to do about it. Skip rationale.
```

### 2. Commit, tag, push

```bash
git add -A
git status   # sanity check
git commit -m "v<next-version> — <one-line summary>"
git tag -a v<next-version> -m "v<next-version> — <release notes>"
git push origin main v<next-version>
```

The release workflow fires automatically on the tag push. Watch it:

```bash
gh run list --workflow=release.yml --limit 3
gh run watch
```

Total runtime ~3–5 minutes (4 build jobs in parallel + one release-assembly job).

> **Note on first-tag-after-workflow-changes:** if you've just pushed a change to `release.yml` along with a tag in the same push, GitHub may not fire the workflow on the tag. If `gh run list` shows zero runs after a couple of minutes, delete and re-push the tag:
> ```bash
> git push origin :refs/tags/v<next-version>
> git push origin v<next-version>
> ```

### 3. Capture checksums

After the workflow completes:

```bash
gh release download v<next-version> -p checksums.txt -O /tmp/ndf-checksums.txt
cat /tmp/ndf-checksums.txt
```

You'll see four sha256 lines, one per artifact. Keep this file open — you need values from it for steps 4 and 5.

### 4. Bump the Homebrew tap

```bash
cd ../homebrew-tap
```

Update `Formula/ndf.rb`:
- `version "<next-version>"`
- Replace each `sha256 "..."` with the matching value from `/tmp/ndf-checksums.txt`:
  - `ndf-darwin-arm64` → `on_macos do; on_arm do; sha256 "..."`
  - `ndf-darwin-amd64` → `on_macos do; on_intel do; sha256 "..."`
  - `ndf-linux-amd64` → `on_linux do; on_intel do; sha256 "..."`

Commit + push:

```bash
git add Formula/ndf.rb
git commit -m "ndf <next-version> — <one-line summary>"
git push origin main
```

### 5. Bump the Scoop bucket

```bash
cd ../scoop-bucket
```

Update `bucket/ndf.json`:
- `"version": "<next-version>"`
- `"url"` field: replace `v<old-version>` with `v<next-version>`
- `"hash"`: paste the `ndf-windows-amd64.exe` sha256 from `/tmp/ndf-checksums.txt`

Validate JSON before commit:

```bash
python3 -m json.tool bucket/ndf.json > /dev/null && echo "json ok"
```

Commit + push:

```bash
git add bucket/ndf.json
git commit -m "ndf <next-version> — <one-line summary>"
git push origin main
```

## Post-release verification

### Homebrew (immediate)

```bash
brew upgrade nandu-org/tap/ndf
ndf version    # → ndf v<next-version>
```

If the tap was previously installed at the old version, `brew upgrade` is enough. Fresh install: `brew install nandu-org/tap/ndf`.

### Scoop (requires Windows)

```powershell
scoop update
scoop update ndf
ndf version    # → ndf v<next-version>
```

Defer if you don't have Windows access; the manifest is correct as long as JSON validates and the sha256 matches the release artifact.

### Canary update (most important)

Run a real `ndf update` against an existing NDF-tracked project:

```bash
cd <some-ndf-project>
ndf update
```

Expected: either "already at v<X>" (no drift) or the regular update flow with diff prompts. Anything else — halt and investigate before announcing the release.

## Rollback / re-tag

Tags are not branches; deleting a remote tag and re-pushing is a normal operation, not a rewrite-history hazard. If the release pipeline produced bad binaries (cross-compile broke, signing step failed, etc.) and the GitHub Release is broken:

```bash
# Delete the GitHub Release first (binaries + release notes go).
gh release delete v<bad-version> --cleanup-tag --yes

# Or, if you want to keep the tag and just re-trigger the workflow:
git push origin :refs/tags/v<bad-version>
git push origin v<bad-version>
```

**Never `git push --force` to `main`.** If a commit on `main` itself needs to be undone, use `git revert` and ship the revert as a new patch release.

If a release is out in the wild (clients have already installed it), don't delete the GitHub Release — ship a fix as a new patch version instead. v2.0.0 → v2.0.1 was exactly this case.

## Distribution channels — current status

| Channel | Status | How to install (end user) |
|---|---|---|
| Direct binary download | ✅ Live, every release | `gh release download` or browser |
| `install.sh` (macOS / Linux) | ✅ Live, follows latest `main` | `curl ... \| bash` |
| `install.ps1` (Windows) | ✅ Live, follows latest `main` | `iwr ... \| iex` |
| Homebrew tap | ✅ Live, manual bump per release | `brew install nandu-org/tap/ndf` |
| Scoop bucket | ✅ Live, manual bump per release | `scoop bucket add nandu ... && scoop install ndf` |
| winget | ⏳ Pending signed binaries | `winget install nandu.ndf` (future) |

The two install scripts (`install.sh`, `install.ps1`) live in this repo and resolve the latest GitHub Release at run time, so no per-release update is needed — they always pick up the newest tag. Homebrew and Scoop need manual bumps because pinning checksums is the whole point of those package managers.

## Code signing — current status

v2.0.x ships unsigned. Both platforms show a one-time prompt on first run (Windows SmartScreen, macOS Gatekeeper). README documents this for end users.

A signed release will:
- Sign the macOS binaries with a **Developer ID Application** certificate from Apple Developer Program (org tier), then notarize via Apple's `notarytool`.
- Sign the Windows binary using **Azure Trusted Signing** (Public Trust certificate profile).

The release workflow will be amended to add signing steps inside the matrix per `matrix.goos`. Until both certs are issued, releases ship unsigned. See `SIGNING.md` (forthcoming, when v2.0.2 ships) for the full pipeline.

## Quick checklist (copy-paste)

```
[ ] Pre-flight: tests pass, vet clean, working tree clean
[ ] Local darwin build: go build, ndf version, ndf help, ndf update on a real project
[ ] Bump version.go to v<next-version>
[ ] CHANGELOG entry written, sanitized, dated
[ ] Commit, tag, push (main + tag together)
[ ] gh run watch — workflow green, all 4 binaries + checksums.txt in release
[ ] Capture checksums from release
[ ] Update Formula/ndf.rb (version + 3 sha256s), commit, push
[ ] Update bucket/ndf.json (version + url + 1 hash), validate JSON, commit, push
[ ] brew upgrade, ndf version, ndf update against a real project
[ ] (Internal) KB Version History entry — see ndf-maintainer skill
```
