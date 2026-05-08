#!/usr/bin/env bash
# ndf — Nandu Development Framework CLI
#
# Subcommands:
#   ndf init        [--token=<framework_pat>] [--fieldnotes-token=<fieldnotes_pat>] [--fieldnotes-repo=<owner/repo>] [--version=<x.y.z>]
#       Scaffold a NEW NDF project. Refuses on existing .ndf.json — use \`ndf login\` to set tokens for an existing project.
#   ndf login       [--token=<framework_pat>] [--fieldnotes-token=<fieldnotes_pat>]
#       Set per-developer credentials. Interactive by default (hidden input); accepts flags for non-interactive use (CI).
#   ndf update      [--version=<x.y.z>] [--latest]
#       Update an existing NDF project to the target framework version.
#   ndf config show
#       Print the resolved config with PATs masked.
#
# After a non-no-op update, ndf prints a team handoff message — a paste-ready
# block summarizing version bump, changes, and what coworkers need to do
# (git pull, merge main, /compact).
#
# Per-developer config: ~/.config/nandu/config.json   (mode 0600)
#   {framework_pat, fieldnotes_pat}
#   (Legacy v1.2.x configs may also have fieldnotes_repo here; v1.3.0+ reads
#   it from each project's .ndf.json first and falls back to config.json.)
# Per-project: <project>/.ndf.json
#   {version, pinned_version, installed_checksums, fieldnotes_repo}
# Env overrides: NDF_GITHUB_TOKEN, NDF_FIELDNOTES_TOKEN  (token-only; repo has no env override)
#
# Source of framework files: nandu-org/nandu-dev-framework  (private GitHub repo)
# Source of this CLI:        nandu-org/nandu-dev-framework-cli  (public)

set -euo pipefail

# ---------- constants ----------

readonly NDF_CLI_VERSION="1.3.1"
readonly NDF_REPO="nandu-org/nandu-dev-framework"
readonly NDF_CONFIG_DIR="${HOME}/.config/nandu"
readonly NDF_CONFIG_FILE="${NDF_CONFIG_DIR}/config.json"
readonly NDF_PROJECT_MARKER=".ndf.json"
readonly NDF_PENDING_MIGRATION=".ndf-pending-migration"
# ---------- output helpers ----------

_die() { echo "ndf: error: $*" >&2; exit 1; }
_warn() { echo "ndf: warn: $*" >&2; }
_info() { echo "ndf: $*" >&2; }
_ok() { echo "ndf: $*"; }

# ---------- OS sanity check ----------

_check_os() {
  case "$(uname -s 2>/dev/null || echo unknown)" in
    Linux*|Darwin*) ;;  # native unix — fine
    MINGW*|MSYS*|CYGWIN*)
      _die "ndf does not run on native Windows shells. Use WSL (Windows Subsystem for Linux), in which Claude Code itself runs anyway." ;;
    *)
      _warn "Untested OS '$(uname -s)'. Proceeding, but expect rough edges." ;;
  esac
}

# ---------- prerequisite check ----------

_check_prereqs() {
  for cmd in curl jq diff sed awk; do
    command -v "$cmd" >/dev/null 2>&1 || _die "missing required command: $cmd"
  done
  if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
    _die "missing sha256 tool: install coreutils (sha256sum) or have shasum available"
  fi
}

_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# ---------- config ----------

_config_get() {
  # _config_get <key>   — prints value or empty string
  local key="$1"
  [[ -f "$NDF_CONFIG_FILE" ]] || { echo ""; return; }
  local val
  if ! val="$(jq -r --arg k "$key" '.[$k] // ""' "$NDF_CONFIG_FILE" 2>/dev/null)"; then
    _die "${NDF_CONFIG_FILE} is not valid JSON. Inspect it (cat ${NDF_CONFIG_FILE}) and fix, or rebuild with: rm ${NDF_CONFIG_FILE} && ndf login"
  fi
  echo "$val"
}

_config_save() {
  # _config_save <framework_pat> <fieldnotes_pat>
  # Per v1.3.0: fieldnotes_repo is per-project (.ndf.json), no longer stored
  # here. Legacy v1.2.x configs that have fieldnotes_repo are preserved
  # (so the slash command's fallback can still find it).
  mkdir -p "$NDF_CONFIG_DIR"
  chmod 700 "$NDF_CONFIG_DIR"

  local existing_repo=""
  if [[ -f "$NDF_CONFIG_FILE" ]]; then
    existing_repo="$(jq -r '.fieldnotes_repo // ""' "$NDF_CONFIG_FILE" 2>/dev/null || echo "")"
  fi

  if [[ -n "$existing_repo" ]]; then
    jq -n \
      --arg framework "$1" \
      --arg fieldnotes "$2" \
      --arg repo "$existing_repo" \
      '{framework_pat: $framework, fieldnotes_pat: $fieldnotes, fieldnotes_repo: $repo}' \
      > "$NDF_CONFIG_FILE"
  else
    jq -n \
      --arg framework "$1" \
      --arg fieldnotes "$2" \
      '{framework_pat: $framework, fieldnotes_pat: $fieldnotes}' \
      > "$NDF_CONFIG_FILE"
  fi
  chmod 600 "$NDF_CONFIG_FILE"
}

_resolve_token() {
  # NDF_GITHUB_TOKEN env var > config file (framework_pat key)
  if [[ -n "${NDF_GITHUB_TOKEN:-}" ]]; then
    echo "$NDF_GITHUB_TOKEN"
  else
    _config_get framework_pat
  fi
}

_resolve_fieldnotes_token() {
  # NDF_FIELDNOTES_TOKEN env var > config file (fieldnotes_pat key)
  if [[ -n "${NDF_FIELDNOTES_TOKEN:-}" ]]; then
    echo "$NDF_FIELDNOTES_TOKEN"
  else
    _config_get fieldnotes_pat
  fi
}

_resolve_fieldnotes_repo() {
  # Per-project marker (.ndf.json) takes precedence; falls back to per-developer
  # config (legacy v1.2.x location). Returns empty string if neither has it.
  if [[ -f "$NDF_PROJECT_MARKER" ]]; then
    local v
    v="$(jq -r '.fieldnotes_repo // ""' "$NDF_PROJECT_MARKER" 2>/dev/null || echo "")"
    if [[ -n "$v" ]]; then
      echo "$v"
      return
    fi
  fi
  _config_get fieldnotes_repo
}

# ---------- HTTP ----------

_curl() {
  # _curl <url> [extra args...]   — outputs body to stdout, fails on non-2xx
  local url="$1"; shift
  local pat
  pat="$(_resolve_token)"
  [[ -n "$pat" ]] || _die "no GitHub PAT configured. Run \`ndf init --token=...\` first."
  curl -fsSL \
    -H "Authorization: token $pat" \
    -H "User-Agent: ndf-cli/${NDF_CLI_VERSION}" \
    "$url" "$@"
}

_fetch_manifest() {
  # _fetch_manifest <ref>  → prints JSON
  local ref="$1"
  _curl "https://raw.githubusercontent.com/${NDF_REPO}/${ref}/manifest.json"
}

_fetch_file_to() {
  # _fetch_file_to <ref> <path-in-repo> <dest-on-disk>
  local ref="$1" path="$2" dest="$3"
  mkdir -p "$(dirname "$dest")"
  _curl "https://raw.githubusercontent.com/${NDF_REPO}/${ref}/${path}" -o "$dest"
}

_resolve_latest_tag() {
  # query the tags API; pick the highest semver tag matching v*.*.*
  local pat
  pat="$(_resolve_token)"
  [[ -n "$pat" ]] || _die "no GitHub PAT configured."
  curl -fsSL \
    -H "Authorization: token $pat" \
    -H "Accept: application/vnd.github+json" \
    -H "User-Agent: ndf-cli/${NDF_CLI_VERSION}" \
    "https://api.github.com/repos/${NDF_REPO}/tags?per_page=100" \
    | jq -r '.[].name' \
    | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
    | sort -t. -k1.2,1n -k2,2n -k3,3n \
    | tail -n 1
}

_resolve_ref() {
  # _resolve_ref <version-or-empty>  → prints "v3.0.0" (with leading v) or "main"
  local v="$1"
  if [[ -z "$v" ]]; then
    local latest
    latest="$(_resolve_latest_tag || true)"
    if [[ -z "$latest" ]]; then
      _warn "no version tags found in repo; falling back to \`main\`"
      echo "main"
    else
      echo "$latest"
    fi
  else
    # strip any leading v then re-add
    v="${v#v}"
    echo "v${v}"
  fi
}

# ---------- semver compare ----------

_semver_lt() {
  # _semver_lt A B  → returns 0 (true) if A < B
  local a="${1#v}" b="${2#v}"
  [[ "$(printf '%s\n%s\n' "$a" "$b" | sort -t. -k1,1n -k2,2n -k3,3n | head -n 1)" == "$a" && "$a" != "$b" ]]
}

_check_min_cli_version() {
  local manifest="$1"
  local mincli
  mincli="$(echo "$manifest" | jq -r '.min_cli_version // "0.0.0"')"
  if _semver_lt "$NDF_CLI_VERSION" "$mincli"; then
    _die "your CLI (v${NDF_CLI_VERSION}) is older than this release requires (min v${mincli}). Re-install: re-run the install one-liner from the onboarding email."
  fi
}

# ---------- prompt ----------

_prompt() {
  # _prompt <question> <default-value>   → echoes user's response (or default if empty)
  local q="$1" default="${2:-}"
  local answer
  if [[ -t 0 ]]; then
    read -r -p "$q " answer
  else
    answer=""
  fi
  echo "${answer:-$default}"
}

# ---------- .ndf.json helpers ----------

_marker_load() {
  # outputs the .ndf.json content; aborts if missing
  [[ -f "$NDF_PROJECT_MARKER" ]] || _die "no ${NDF_PROJECT_MARKER} found in current directory. This isn't an ndf project — run \`ndf init\` first."
  cat "$NDF_PROJECT_MARKER"
}

_marker_get() {
  # _marker_get <key>
  local key="$1"
  [[ -f "$NDF_PROJECT_MARKER" ]] || { echo ""; return; }
  jq -r --arg k "$key" '.[$k] // ""' "$NDF_PROJECT_MARKER"
}

_marker_write() {
  # _marker_write <version> <pinned_or_null> <checksums-jq-object> [fieldnotes_repo]
  # If fieldnotes_repo is absent or empty, preserve any existing value in the
  # marker (so a no-op update doesn't drop it). If present, set it.
  local version="$1" pinned="$2" checksums="$3" repo="${4:-}"

  local existing_repo=""
  if [[ -f "$NDF_PROJECT_MARKER" ]]; then
    existing_repo="$(jq -r '.fieldnotes_repo // ""' "$NDF_PROJECT_MARKER" 2>/dev/null || echo "")"
  fi
  [[ -z "$repo" ]] && repo="$existing_repo"

  local pinned_arg
  if [[ "$pinned" == "null" ]]; then
    pinned_arg='null'
  else
    pinned_arg="\"$pinned\""
  fi

  if [[ -n "$repo" ]]; then
    jq -n \
      --arg v "$version" \
      --argjson c "$checksums" \
      --arg repo "$repo" \
      --argjson p "$pinned_arg" \
      '{version: $v, pinned_version: $p, installed_checksums: $c, fieldnotes_repo: $repo}' \
      > "$NDF_PROJECT_MARKER"
  else
    jq -n \
      --arg v "$version" \
      --argjson c "$checksums" \
      --argjson p "$pinned_arg" \
      '{version: $v, pinned_version: $p, installed_checksums: $c}' \
      > "$NDF_PROJECT_MARKER"
  fi
}

# ---------- subcommand: init ----------

cmd_init() {
  local cli_framework_pat="" cli_fieldnotes_pat="" cli_fieldnotes_repo="" requested_version=""
  for arg in "$@"; do
    case "$arg" in
      --token=*) cli_framework_pat="${arg#*=}" ;;
      --fieldnotes-token=*) cli_fieldnotes_pat="${arg#*=}" ;;
      --fieldnotes-repo=*) cli_fieldnotes_repo="${arg#*=}" ;;
      --version=*) requested_version="${arg#*=}" ;;
      -h|--help) _print_help_init; return 0 ;;
      *) _die "unknown init flag: $arg" ;;
    esac
  done

  # ndf init scaffolds a NEW project. If .ndf.json exists, redirect to the
  # right command for the user's intent.
  if [[ -f "$NDF_PROJECT_MARKER" ]]; then
    _die "$(printf '%s\n' \
      "${NDF_PROJECT_MARKER} already exists. This is already an NDF project." \
      "" \
      "  To set or update your credentials:  ndf login" \
      "  To update the project:              ndf update")"
  fi

  # If tokens were provided as flags, persist them now (before the project
  # scaffolding work) so a partial scaffold leaves credentials configured.
  if [[ -n "$cli_framework_pat" || -n "$cli_fieldnotes_pat" ]]; then
    local existing_framework existing_fieldnotes
    existing_framework="$(_config_get framework_pat)"
    existing_fieldnotes="$(_config_get fieldnotes_pat)"

    [[ -n "$cli_framework_pat" ]] && existing_framework="$cli_framework_pat"
    [[ -n "$cli_fieldnotes_pat" ]] && existing_fieldnotes="$cli_fieldnotes_pat"

    _config_save "$existing_framework" "$existing_fieldnotes"
    _info "tokens saved to ${NDF_CONFIG_FILE} (mode 0600)"
  fi

  # Verify we have a framework token at this point.
  [[ -n "$(_resolve_token)" ]] || _die "no framework PAT configured. Run \`ndf login\` first, or pass --token=<ghp_xxx>."

  # Warn if /field-note will be inoperative on this machine
  if [[ -z "$(_resolve_fieldnotes_token)" ]]; then
    _warn "no field-notes PAT configured. /field-note will not work until you run \`ndf login\` with the field-notes token."
  fi

  local ref
  ref="$(_resolve_ref "$requested_version")"
  _info "fetching manifest for ${ref}…"
  local manifest
  manifest="$(_fetch_manifest "$ref")"
  _check_min_cli_version "$manifest"

  local mver
  mver="$(echo "$manifest" | jq -r '.version')"
  _info "installing v${mver}"

  # Fetch every file in the manifest.
  local checksums="{}"
  while IFS=$'\t' read -r path expected_sha; do
    _info "  ${path}"
    _fetch_file_to "$ref" "$path" "$path"
    local got
    got="$(_sha256 "$path")"
    if [[ "$got" != "$expected_sha" ]]; then
      _die "checksum mismatch on ${path}: expected ${expected_sha}, got ${got}. Aborting init; rerun or escalate."
    fi
    checksums="$(echo "$checksums" | jq --arg p "$path" --arg c "$got" '. + {($p): $c}')"
  done < <(echo "$manifest" | jq -r '.files[] | "\(.path)\t\(.checksum)"')

  # Make hook script executable if present.
  [[ -f .claude/hooks/pre-commit-tests.sh ]] && chmod +x .claude/hooks/pre-commit-tests.sh

  # Create the project-owned CLAUDE.project.md stub if not already present.
  # (It's intentionally NOT in the manifest — client owns it forever.)
  if [[ ! -f CLAUDE.project.md ]]; then
    cat > CLAUDE.project.md <<'STUB'
# <project-name>

## Stack
- Language: <e.g. Python 3.12, Node.js 20, Rust 1.75>
- Framework: <e.g. FastAPI, Express, Actix>
- Database: <e.g. BigQuery, Postgres, SQLite>
- Dependency cache path: <e.g. `node_modules/`, `.venv/lib/python3.12/site-packages/`, `~/.cargo/registry/src/`> — used by the inspect-over-execute rule for `Read`/`Grep` on installed library source.
- <Other critical dependencies>

## Key rules
- <non-negotiable constraint — e.g. "all data access must be tenant-scoped">

## Verification commands
```bash
<test command>   # unit tests
<lint command>   # lint
<type command>   # type check
```

## Required tooling
<MCPs your project depends on; e.g. context7 for stack documentation>
STUB
    _info "created CLAUDE.project.md stub (you own this file — fill it in)"
  fi

  # Pin? If --version was passed, set pinned_version.
  local pinned="null"
  [[ -n "$requested_version" ]] && pinned="$mver"

  _marker_write "$mver" "$pinned" "$checksums" "$cli_fieldnotes_repo"
  _ok "ndf init complete. Installed v${mver} into $(pwd)."
  if [[ -n "$cli_fieldnotes_repo" ]]; then
    _info "fieldnotes_repo set to ${cli_fieldnotes_repo} in .ndf.json — commit this so coworkers pick it up automatically."
  else
    _warn "no --fieldnotes-repo provided; /field-note won't have a target until it's set in .ndf.json (or fall back to ~/.config/nandu/config.json)."
  fi
  _ok "Next steps: edit CLAUDE.project.md, .claude/hooks/pre-commit-tests.sh, and .claude/settings.json (allow-list) per METHODOLOGY.md."
}

# ---------- subcommand: update ----------

cmd_update() {
  local requested_version="" use_latest=""
  for arg in "$@"; do
    case "$arg" in
      --version=*) requested_version="${arg#*=}" ;;
      --latest) use_latest=1 ;;
      -h|--help) _print_help_update; return 0 ;;
      *) _die "unknown update flag: $arg" ;;
    esac
  done
  if [[ -n "$requested_version" && -n "$use_latest" ]]; then
    _die "--version and --latest are mutually exclusive."
  fi

  # Friendly message if tokens aren't configured (better than letting _curl
  # fail with a generic "no GitHub PAT" error mid-flow).
  if [[ -z "$(_resolve_token)" ]]; then
    _die "$(printf '%s\n' \
      "no framework PAT configured." \
      "" \
      "Run \`ndf login\` to set your tokens, then re-run \`ndf update\`.")"
  fi

  _marker_load >/dev/null
  local current_version pinned_version
  current_version="$(_marker_get version)"
  pinned_version="$(_marker_get pinned_version)"

  # Determine target version.
  local target=""
  if [[ -n "$requested_version" ]]; then
    target="$requested_version"
  elif [[ -n "$use_latest" ]]; then
    target=""  # resolved to latest tag below
  elif [[ -n "$pinned_version" && "$pinned_version" != "null" ]]; then
    target="$pinned_version"
  fi

  local ref
  ref="$(_resolve_ref "$target")"
  _info "fetching manifest for ${ref}…"
  local manifest
  manifest="$(_fetch_manifest "$ref")"
  _check_min_cli_version "$manifest"

  local target_version
  target_version="$(echo "$manifest" | jq -r '.version')"

  if [[ "$target_version" == "$current_version" ]]; then
    _info "already at v${current_version}; checking for drift…"
  else
    _info "updating from v${current_version} to v${target_version}"
  fi

  # Migrations gate: if this manifest declares pending migrations, pre-deliver only those + the slash command, then stop.
  # Sentinel-aware: each migration in manifest.migrations[] is considered applied when
  # `.ndf-migrations/<migration-name>.complete` exists. If every listed migration is
  # already applied, skip the gate entirely and proceed to the regular file-level update.
  # The sentinel-name-matches-migration-name convention is enforced by migration specs
  # (see ndf-maintainer skill).
  local migration_count
  migration_count="$(echo "$manifest" | jq -r '.migrations // [] | length')"
  if [[ "$migration_count" -gt 0 ]]; then
    local migration_list
    migration_list="$(echo "$manifest" | jq -r '.migrations[]')"

    # Sentinel check — count migrations still pending (no .complete file yet).
    local pending_migrations=()
    while IFS= read -r migration_name; do
      [[ -z "$migration_name" ]] && continue
      if [[ ! -f ".ndf-migrations/${migration_name}.complete" ]]; then
        pending_migrations+=("$migration_name")
      fi
    done <<< "$migration_list"

    if [[ ${#pending_migrations[@]} -eq 0 ]]; then
      _info "all ${migration_count} migration(s) in manifest already applied (sentinels present); skipping migration gate."
      # Clean up any stale pending-migration marker from older CLI versions.
      rm -f "$NDF_PENDING_MIGRATION"
      # Fall through to the regular file-level update flow below.
    else
      _info "this release includes ${#pending_migrations[@]} pending structural migration(s); pre-delivering specs…"

      # Pre-deliver each pending migration spec and the slash command.
      for migration_name in "${pending_migrations[@]}"; do
        local spec_path="migrations/${migration_name}.md"
        _info "  ${spec_path}"
        _fetch_file_to "$ref" "$spec_path" "$spec_path"
      done

      _info "  .claude/commands/ndf-migrate.md"
      _fetch_file_to "$ref" ".claude/commands/ndf-migrate.md" ".claude/commands/ndf-migrate.md"

      # Write the pending-migration marker so /ndf-migrate knows what to apply.
      printf "%s\n" "${pending_migrations[@]}" > "$NDF_PENDING_MIGRATION"

      _ok ""
      _ok "v${target_version} includes a structural migration. Run /ndf-migrate in Claude Code to apply,"
      _ok "then re-run \`ndf update\` to complete the file-level changes."
      return 0
    fi
  fi

  # Build a lookup of new-manifest entries.
  local new_paths_json
  new_paths_json="$(echo "$manifest" | jq '[.files[] | {path: .path, checksum: .checksum, renamed_from: (.renamed_from // null)}]')"

  # Old manifest = installed_checksums in .ndf.json (path → checksum)
  local installed_json
  installed_json="$(_marker_load | jq '.installed_checksums')"

  # Track changes for the team handoff message at the end.
  local changes_file
  changes_file="$(mktemp)"

  # ---- pass 1: process each file in the new manifest ----
  while IFS= read -r entry; do
    local p new_sha rn
    p="$(echo "$entry" | jq -r '.path')"
    new_sha="$(echo "$entry" | jq -r '.checksum')"
    rn="$(echo "$entry" | jq -r '.renamed_from // ""')"

    if [[ -n "$rn" ]]; then
      # Renamed file
      local old_installed_sha
      old_installed_sha="$(echo "$installed_json" | jq -r --arg k "$rn" '.[$k] // ""')"
      if [[ -z "$old_installed_sha" ]]; then
        _warn "rename ${rn} → ${p}: source not found in installed manifest. Treating as a new file."
        _fetch_file_to "$ref" "$p" "$p"
      else
        local current_sha=""
        [[ -f "$rn" ]] && current_sha="$(_sha256 "$rn")"
        if [[ "$current_sha" == "$old_installed_sha" ]]; then
          # Client did not modify; safe rename.
          _info "  rename: ${rn} → ${p}"
          mkdir -p "$(dirname "$p")"
          _fetch_file_to "$ref" "$p" "$p"
          [[ -f "$rn" ]] && rm "$rn"
          echo "rename	${rn} → ${p}" >> "$changes_file"
        else
          _warn "  rename ${rn} → ${p}: client has modified ${rn}; downloading framework version to ${p} for review."
          _fetch_file_to "$ref" "$p" "$p"
          _info "  diff (yours vs framework, on the new path):"
          diff -u "$rn" "$p" || true
          local choice
          choice="$(_prompt "  [r]eplace ${rn}'s changes with framework version, [s]kip rename, or [b]ackup-and-replace? (default: s)" "s")"
          case "$choice" in
            r|R) rm -f "$rn"; _info "    ${rn} removed; framework content lives at ${p} now." ;;
            b|B) cp "$rn" "${rn}.local-backup"; rm -f "$rn"; _info "    backed up ${rn} → ${rn}.local-backup; framework content at ${p}." ;;
            *) rm "$p"; _info "    rename skipped; ${rn} preserved (framework version at ${p} removed)." ;;
          esac
        fi
      fi
    elif ! grep -q "\"$p\"" <<< "$installed_json"; then
      # Net-new file
      _info "  new: ${p}"
      _fetch_file_to "$ref" "$p" "$p"
      echo "new	${p}" >> "$changes_file"
    else
      # Existing file — check for content change
      local installed_sha
      installed_sha="$(echo "$installed_json" | jq -r --arg k "$p" '.[$k] // ""')"
      if [[ "$installed_sha" == "$new_sha" ]]; then
        # Framework hasn't changed it; nothing to do.
        :
      else
        local current_sha=""
        [[ -f "$p" ]] && current_sha="$(_sha256 "$p")"
        if [[ ! -f "$p" ]]; then
          _warn "  ${p}: file missing locally; restoring from framework."
          _fetch_file_to "$ref" "$p" "$p"
        elif [[ "$current_sha" == "$installed_sha" ]]; then
          # Client hasn't modified; replace silently.
          _info "  update: ${p}"
          _fetch_file_to "$ref" "$p" "$p"
          echo "update	${p}" >> "$changes_file"
        else
          # Both client AND framework changed it — diff and prompt.
          local tmp
          tmp="$(mktemp)"
          _fetch_file_to "$ref" "$p" "$tmp"
          _warn "  ${p}: changed both locally and upstream."
          diff -u "$p" "$tmp" || true
          local choice
          choice="$(_prompt "  [r]eplace with framework, [s]kip, or [b]ackup-and-replace? (default: s)" "s")"
          case "$choice" in
            r|R) cp "$tmp" "$p"; _info "    replaced ${p}."; echo "update-with-conflict	${p}" >> "$changes_file" ;;
            b|B) cp "$p" "${p}.local-backup"; cp "$tmp" "$p"; _info "    backed up ${p} → ${p}.local-backup; replaced with framework version."; echo "update-with-conflict	${p}" >> "$changes_file" ;;
            *) _info "    skipped ${p}." ;;
          esac
          rm -f "$tmp"
        fi
      fi
    fi

  done < <(echo "$new_paths_json" | jq -c '.[]')

  # ---- build new_checksums from the manifest, not from disk ----
  local new_checksums
  # The marker should reflect the FRAMEWORK VERSIONS we presented to the user,
  # not on-disk state. Disk state can drift (customizations); recording disk
  # state in installed_checksums causes a customized-then-skipped file to be
  # silently reverted on the next run (manifest_sha != "installed_sha", but
  # current_sha == "installed_sha", which the diff logic interprets as
  # "framework changed; client unchanged" → silent replace, destroying the
  # customization). Always set new_checksums = {path: manifest_sha} for every
  # file in the new manifest. Renamed files: only the new path is in the
  # manifest, so the old path is naturally absent. Removed files are absent
  # from the manifest and therefore from new_checksums.
  new_checksums="$(echo "$new_paths_json" | jq 'map({(.path): .checksum}) | add // {}')"

  # ---- pass 2: removed files (in old, not in new, not as a rename source) ----
  # Build a set of paths from new manifest (including rename sources we already handled)
  local renamed_sources
  renamed_sources="$(echo "$new_paths_json" | jq -r '[.[] | select(.renamed_from != null) | .renamed_from] | join("\t")')"
  echo "$installed_json" | jq -r 'keys[]' | while IFS= read -r old_path; do
    if ! echo "$new_paths_json" | jq -e --arg p "$old_path" 'any(.[]; .path == $p)' > /dev/null; then
      # not in new manifest
      if echo "$renamed_sources" | tr '\t' '\n' | grep -qx "$old_path"; then
        :  # already handled as a rename source
      else
        if [[ -f "$old_path" ]]; then
          _warn "removing ${old_path} (no longer in framework)"
          rm "$old_path"
          echo "remove	${old_path}" >> "$changes_file"
        fi
      fi
    fi
  done

  # ---- update .ndf.json ----
  # If we used --latest, clear the pin; if --version, set pin to that version; else preserve pin.
  local new_pinned="null"
  if [[ -n "$requested_version" ]]; then
    new_pinned="$target_version"
  elif [[ -n "$use_latest" ]]; then
    new_pinned="null"
  elif [[ -n "$pinned_version" && "$pinned_version" != "null" ]]; then
    new_pinned="$pinned_version"
  fi
  # Preserve existing fieldnotes_repo through the update (if any).
  local existing_repo
  existing_repo="$(jq -r '.fieldnotes_repo // ""' "$NDF_PROJECT_MARKER" 2>/dev/null || echo "")"
  _marker_write "$target_version" "$new_pinned" "$new_checksums" "$existing_repo"

  _ok "ndf update complete. Now at v${target_version}."

  # ---- offer commit + push BEFORE the team handoff message ----
  _offer_commit_and_push "$target_version" "$changes_file" "$migration_count"

  # ---- team handoff message ----
  _print_team_handoff "$current_version" "$target_version" "$changes_file" "$migration_count"
  rm -f "$changes_file"
}

# Offer to commit + push the framework changes before showing the team handoff.
# The handoff message tells coworkers to git pull, so it's premature if the
# updater hasn't pushed yet.
_offer_commit_and_push() {
  local target_version="$1" cf="$2" migration_count="${3:-0}"
  local proj_dir="${CLAUDE_PROJECT_DIR:-$(pwd)}"

  # Skip if not in a git repo (e.g., a throwaway test dir)
  if ! git -C "$proj_dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    return 0
  fi

  # Skip if the update was a no-op (no changes_file content and no migrations)
  if [[ ! -s "$cf" ]] && [[ "$migration_count" -eq 0 ]]; then
    return 0
  fi

  local branch
  branch="$(git -C "$proj_dir" branch --show-current 2>/dev/null)"

  # Skip if no uncommitted changes (already committed somehow)
  if git -C "$proj_dir" diff --quiet && git -C "$proj_dir" diff --cached --quiet; then
    return 0
  fi

  # Warn if not on a typical integration branch
  if [[ "$branch" != "main" && "$branch" != "master" ]]; then
    _warn "you are on branch \"$branch\", not the typical integration branch."
    _warn "Framework updates usually land on main first; coworkers pull from there."
  fi

  echo ""
  echo "Framework changes are uncommitted on branch \"$branch\"."
  echo "Coworkers will pull from main, so push before sharing the team handoff."
  echo ""

  local choice
  choice="$(_prompt "Commit and push these changes now? [Y/n]" "y")"
  case "$choice" in
    n|N|no|NO|skip|SKIP)
      _warn "Skipped. Before pasting the team handoff into chat:"
      _warn "  git -C \"$proj_dir\" add -A"
      _warn "  git -C \"$proj_dir\" commit -m \"ndf: update to v${target_version}\""
      _warn "  git -C \"$proj_dir\" push origin ${branch}"
      return 0
      ;;
  esac

  # Default branch is yes — commit + push
  if ! git -C "$proj_dir" add -A; then
    _warn "git add failed; commit yourself manually before sharing handoff."
    return 1
  fi

  if ! git -C "$proj_dir" commit -m "ndf: update to v${target_version}"; then
    _warn "git commit failed; resolve and commit manually before sharing handoff."
    return 1
  fi
  _info "committed: ndf: update to v${target_version}"

  if ! git -C "$proj_dir" push origin "${branch}" 2>&1; then
    _warn "git push failed; the commit landed locally but is NOT yet on remote."
    _warn "Push manually before sharing the team handoff message."
    return 1
  fi
  _ok "pushed to origin/${branch}"
}

# Print a paste-ready team handoff message after a non-no-op update.
_print_team_handoff() {
  local from_v="$1" to_v="$2" cf="$3" migration_count="${4:-0}"

  # Skip the message entirely if nothing changed.
  if [[ ! -s "$cf" ]] && [[ "$migration_count" -eq 0 ]]; then
    return 0
  fi

  echo ""
  echo "===================="
  echo "TEAM HANDOFF — paste in your team chat"
  echo "===================="
  echo ""
  if [[ "$from_v" == "$to_v" ]]; then
    echo "Framework updated on main (drift fixes; version stays at v${to_v})"
  else
    echo "Framework updated: v${from_v} → v${to_v} (on main)"
  fi
  echo ""

  if [[ "$migration_count" -gt 0 ]]; then
    echo "⚠️  THIS UPDATE INCLUDES A STRUCTURAL MIGRATION."
    echo "   After merging main, run /ndf-migrate in Claude Code, then continue."
    echo ""
  fi

  if [[ -s "$cf" ]]; then
    local total
    total="$(wc -l < "$cf" | tr -d ' ')"
    echo "Changed (${total} file(s)):"
    # Show each change with its kind prefix
    awk -F'	' '{print "- " $2 " (" $1 ")"}' "$cf"
    echo ""
  fi

  echo "What you need to do:"
  echo "- git pull origin main"
  echo "- If on a phase branch: git merge main (or rebase per team convention)"
  if [[ "$migration_count" -gt 0 ]]; then
    echo "- Run /ndf-migrate in your Claude Code session"
  fi
  echo "- If you have an active Claude Code session: /compact after merging"
  echo ""
  echo "CHANGELOG: https://github.com/${NDF_REPO}/blob/main/CHANGELOG.md"
  echo ""
  echo "===================="
}

# ---------- subcommand: login ----------

cmd_login() {
  local cli_framework_pat="" cli_fieldnotes_pat=""
  for arg in "$@"; do
    case "$arg" in
      --token=*) cli_framework_pat="${arg#*=}" ;;
      --fieldnotes-token=*) cli_fieldnotes_pat="${arg#*=}" ;;
      -h|--help) _print_help_login; return 0 ;;
      *) _die "unknown login flag: $arg" ;;
    esac
  done

  # Existing values (so user can press Enter to keep current)
  local existing_framework existing_fieldnotes
  existing_framework="$(_config_get framework_pat)"
  existing_fieldnotes="$(_config_get fieldnotes_pat)"

  local new_framework="$cli_framework_pat"
  local new_fieldnotes="$cli_fieldnotes_pat"

  # Interactive prompt for framework PAT (if not provided as flag)
  if [[ -z "$new_framework" ]]; then
    local prompt_label="Framework PAT"
    [[ -n "$existing_framework" ]] && prompt_label="${prompt_label} [press Enter to keep current]"
    printf "%s: " "$prompt_label" >&2
    read -rs new_framework
    echo "" >&2
    [[ -z "$new_framework" ]] && new_framework="$existing_framework"
  fi

  # Interactive prompt for fieldnotes PAT
  if [[ -z "$new_fieldnotes" ]]; then
    local prompt_label="Field-notes PAT"
    if [[ -n "$existing_fieldnotes" ]]; then
      prompt_label="${prompt_label} [press Enter to keep current]"
    else
      prompt_label="${prompt_label} (leave empty if not yet provisioned)"
    fi
    printf "%s: " "$prompt_label" >&2
    read -rs new_fieldnotes
    echo "" >&2
    [[ -z "$new_fieldnotes" ]] && new_fieldnotes="$existing_fieldnotes"
  fi

  # Validate framework PAT (required)
  if [[ -z "$new_framework" ]]; then
    _die "framework PAT is required. Get yours from your team's secure credential share."
  fi

  _config_save "$new_framework" "$new_fieldnotes"
  _ok "tokens saved to ${NDF_CONFIG_FILE} (mode 0600)"

  if [[ -z "$new_fieldnotes" ]]; then
    _warn "field-notes PAT not set. /field-note will not work until you re-run \`ndf login\` with both tokens."
  fi
}

# ---------- subcommand: config ----------

cmd_config() {
  local sub="${1:-}"
  shift || true
  case "$sub" in
    show) _config_show "$@" ;;
    set)  _die "\`ndf config set\` is not supported. To set tokens, run \`ndf login\`. fieldnotes_repo is per-project — set it via \`ndf init --fieldnotes-repo=...\` from a fresh project, or edit the project's .ndf.json directly." ;;
    -h|--help|help|"") _print_help_config ;;
    *) _die "unknown config subcommand: $sub. Try: ndf config show" ;;
  esac
}

_config_show() {
  if [[ ! -f "$NDF_CONFIG_FILE" ]]; then
    echo "No config at ${NDF_CONFIG_FILE}. Run \`ndf login\` to set up credentials."
    return 0
  fi

  local framework fieldnotes legacy_repo
  framework="$(_config_get framework_pat)"
  fieldnotes="$(_config_get fieldnotes_pat)"
  legacy_repo="$(_config_get fieldnotes_repo)"

  echo "Per-developer config (${NDF_CONFIG_FILE}):"
  echo "  framework_pat:  $(_mask_token "$framework")"
  echo "  fieldnotes_pat: $(_mask_token "$fieldnotes")"
  if [[ -n "$legacy_repo" ]]; then
    echo "  fieldnotes_repo: ${legacy_repo}  (legacy v1.2.x location; v1.3.0+ reads per-project .ndf.json first)"
  fi
  echo ""

  if [[ -f "$NDF_PROJECT_MARKER" ]]; then
    echo "Per-project marker (./${NDF_PROJECT_MARKER}):"
    local proj_version proj_pinned proj_repo
    proj_version="$(jq -r '.version // "(unknown)"' "$NDF_PROJECT_MARKER" 2>/dev/null)"
    proj_pinned="$(jq -r '.pinned_version // "null"' "$NDF_PROJECT_MARKER" 2>/dev/null)"
    proj_repo="$(jq -r '.fieldnotes_repo // ""' "$NDF_PROJECT_MARKER" 2>/dev/null)"
    echo "  version:         ${proj_version}"
    echo "  pinned_version:  ${proj_pinned}"
    if [[ -n "$proj_repo" ]]; then
      echo "  fieldnotes_repo: ${proj_repo}"
    fi
  else
    echo "(not currently in an NDF project — no .ndf.json in cwd)"
  fi

  echo ""
  echo "Resolved fieldnotes_repo (for /field-note in this directory):"
  local resolved
  resolved="$(_resolve_fieldnotes_repo)"
  if [[ -n "$resolved" ]]; then
    echo "  ${resolved}"
  else
    echo "  (not configured — /field-note will fail in this directory)"
  fi
}

_mask_token() {
  local t="$1"
  if [[ -z "$t" ]]; then
    echo "(not set)"
  elif [[ ${#t} -le 8 ]]; then
    echo "***"
  else
    echo "${t:0:4}...${t: -4}"
  fi
}

# ---------- help ----------

_print_help_init() {
  cat <<EOF
Usage: ndf init [flags]

Scaffold a NEW ndf project in the current directory.
Refuses on existing .ndf.json — use \`ndf login\` to set tokens for an existing project,
or \`ndf update\` to update an already-installed project.

Flags:
  --token=<framework_pat>              GitHub PAT, read-only on the framework repo
  --fieldnotes-token=<fieldnotes_pat>  GitHub PAT, write-only on the client's field-notes repo
  --fieldnotes-repo=<owner/repo>       The client's field-notes repo (written to .ndf.json)
  --version=<x.y.z>                    Pin to a specific framework version (default: latest tag)

Tokens (--token, --fieldnotes-token) are persisted to ~/.config/nandu/config.json
(per-developer). The fieldnotes_repo is persisted to the project's .ndf.json
(per-project, committed) so coworkers cloning the project pick it up automatically.

Env vars NDF_GITHUB_TOKEN and NDF_FIELDNOTES_TOKEN override the config file when set.
fieldnotes_repo has no env-var override.

To set credentials WITHOUT scaffolding a project, use \`ndf login\` instead.
EOF
}

_print_help_login() {
  cat <<EOF
Usage: ndf login [flags]

Set per-developer credentials. Interactive by default — prompts for tokens
with hidden input (the values don't appear on screen or in shell history).

Flags (for non-interactive / CI use):
  --token=<framework_pat>              Framework PAT (read-only on the framework repo)
  --fieldnotes-token=<fieldnotes_pat>  Field-notes PAT (write-only on the client's field-notes repo)

If a flag is provided, that value is used directly (no prompt). If a flag is
omitted, the prompt offers the existing value (press Enter to keep it).

Tokens are saved to ~/.config/nandu/config.json (mode 0600). Use \`ndf config
show\` to verify the resolved state without exposing the raw values.
EOF
}

_print_help_config() {
  cat <<EOF
Usage: ndf config <subcommand> [flags]

Subcommands:
  show    Print the resolved per-developer + per-project config (PATs masked)

To set tokens: \`ndf login\`.
To set the fieldnotes_repo for a project: \`ndf init --fieldnotes-repo=...\`
from a fresh directory, or edit the project's .ndf.json directly.
EOF
}

_print_help_update() {
  cat <<EOF
Usage: ndf update [flags]

Update the framework files in the current ndf project.

Flags:
  --version=<x.y.z>   Set the project's pinned_version to X and update to it.
  --latest            Clear the pinned_version and update to the latest tag.
                      (Mutually exclusive with --version.)

With no flags, updates to the project's pinned_version (or latest if no pin).
EOF
}

_print_help() {
  cat <<EOF
ndf — Nandu Development Framework CLI (v${NDF_CLI_VERSION})

Usage: ndf <command> [flags]

Commands:
  init           Scaffold a NEW ndf project in the current directory
  login          Set per-developer credentials (interactive by default)
  update         Update an existing ndf project to a target framework version
  config show    Print the resolved config (per-developer + per-project), PATs masked
  version        Print the CLI version
  help           Print this help

Run \`ndf <command> --help\` for command-specific help.

Typical onboarding flow for joining an existing NDF project:
  1) Install the CLI via the install.sh one-liner
  2) ndf login                          (set your tokens — interactive, hidden input)
  3) cd <project> && ndf update         (verify everything works)
EOF
}

# ---------- main ----------

main() {
  _check_os
  _check_prereqs
  local cmd="${1:-help}"
  shift || true
  case "$cmd" in
    init)    cmd_init "$@" ;;
    login)   cmd_login "$@" ;;
    update)  cmd_update "$@" ;;
    config)  cmd_config "$@" ;;
    version) echo "ndf v${NDF_CLI_VERSION}" ;;
    help|-h|--help) _print_help ;;
    *) _print_help; exit 1 ;;
  esac
}

main "$@"
