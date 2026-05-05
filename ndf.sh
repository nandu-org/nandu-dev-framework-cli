#!/usr/bin/env bash
# ndf — Nandu Development Framework CLI
#
# Subcommands:
#   ndf init   [--token=<framework_pat>] [--fieldnotes-token=<fieldnotes_pat>] [--fieldnotes-repo=<owner/repo>] [--version=<x.y.z>]
#   ndf update [--version=<x.y.z>] [--latest]
#
# Config: ~/.config/nandu/config.json   (mode 0600)
#   {framework_pat, fieldnotes_pat, fieldnotes_repo}
# Env overrides: NDF_GITHUB_TOKEN, NDF_FIELDNOTES_TOKEN  (token-only; repo has no env override)
#
# Source of framework files: nandu-org/nandu-dev-framework  (private GitHub repo)
# Source of this CLI:        nandu-org/nandu-dev-framework-cli  (public)

set -euo pipefail

# ---------- constants ----------

readonly NDF_CLI_VERSION="1.1.0"
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
  jq -r --arg k "$key" '.[$k] // ""' "$NDF_CONFIG_FILE"
}

_config_save() {
  # _config_save <framework_pat> <fieldnotes_pat> <fieldnotes_repo>
  mkdir -p "$NDF_CONFIG_DIR"
  chmod 700 "$NDF_CONFIG_DIR"
  jq -n \
    --arg framework "$1" \
    --arg fieldnotes "$2" \
    --arg repo "$3" \
    '{framework_pat: $framework, fieldnotes_pat: $fieldnotes, fieldnotes_repo: $repo}' \
    > "$NDF_CONFIG_FILE"
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
  # _marker_write <version> <pinned_or_null> <checksums-jq-object>
  local version="$1" pinned="$2" checksums="$3"
  if [[ "$pinned" == "null" ]]; then
    jq -n --arg v "$version" --argjson c "$checksums" \
      '{version: $v, pinned_version: null, installed_checksums: $c}' \
      > "$NDF_PROJECT_MARKER"
  else
    jq -n --arg v "$version" --arg p "$pinned" --argjson c "$checksums" \
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

  # If any config field was provided on the command line, persist them.
  # Existing fields not overridden by the CLI flags are preserved.
  if [[ -n "$cli_framework_pat" || -n "$cli_fieldnotes_pat" || -n "$cli_fieldnotes_repo" ]]; then
    local existing_framework existing_fieldnotes existing_repo
    existing_framework="$(_config_get framework_pat)"
    existing_fieldnotes="$(_config_get fieldnotes_pat)"
    existing_repo="$(_config_get fieldnotes_repo)"

    [[ -n "$cli_framework_pat" ]] && existing_framework="$cli_framework_pat"
    [[ -n "$cli_fieldnotes_pat" ]] && existing_fieldnotes="$cli_fieldnotes_pat"
    [[ -n "$cli_fieldnotes_repo" ]] && existing_repo="$cli_fieldnotes_repo"

    if [[ -f "$NDF_CONFIG_FILE" ]]; then
      _info "overwriting existing config at ${NDF_CONFIG_FILE}"
    fi
    _config_save "$existing_framework" "$existing_fieldnotes" "$existing_repo"
    _info "wrote ${NDF_CONFIG_FILE} (mode 0600)"

    # Warn if the /field-note slash command will not work due to missing config.
    if [[ -z "$existing_fieldnotes" || -z "$existing_repo" ]]; then
      _warn "--fieldnotes-token and/or --fieldnotes-repo not configured."
      _warn "/field-note will not work until you re-run \`ndf init\` with those flags."
    fi
  fi

  # Verify we have a token at this point.
  [[ -n "$(_resolve_token)" ]] || _die "no GitHub PAT configured. Re-run with --token=<ghp_xxx>."

  # If marker already exists in this directory, refuse.
  if [[ -f "$NDF_PROJECT_MARKER" ]]; then
    _die "${NDF_PROJECT_MARKER} already exists. This is already an ndf project. Use \`ndf update\` instead."
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

  _marker_write "$mver" "$pinned" "$checksums"
  _ok "ndf init complete. Installed v${mver} into $(pwd)."
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
  local migration_count
  migration_count="$(echo "$manifest" | jq -r '.migrations // [] | length')"
  if [[ "$migration_count" -gt 0 ]]; then
    _info "this release includes ${migration_count} structural migration(s); pre-delivering specs…"
    local migration_list
    migration_list="$(echo "$manifest" | jq -r '.migrations[]')"

    # Pre-deliver each migration spec and the slash command.
    while IFS= read -r migration_name; do
      local spec_path="migrations/${migration_name}.md"
      _info "  ${spec_path}"
      _fetch_file_to "$ref" "$spec_path" "$spec_path"
    done <<< "$migration_list"

    _info "  .claude/commands/ndf-migrate.md"
    _fetch_file_to "$ref" ".claude/commands/ndf-migrate.md" ".claude/commands/ndf-migrate.md"

    # Write the pending-migration marker so /ndf-migrate knows what to apply.
    echo "$migration_list" > "$NDF_PENDING_MIGRATION"

    _ok ""
    _ok "v${target_version} includes a structural migration. Run /ndf-migrate in Claude Code to apply,"
    _ok "then re-run \`ndf update\` to complete the file-level changes."
    return 0
  fi

  # Build a lookup of new-manifest entries.
  local new_paths_json
  new_paths_json="$(echo "$manifest" | jq '[.files[] | {path: .path, checksum: .checksum, renamed_from: (.renamed_from // null)}]')"

  # Old manifest = installed_checksums in .ndf.json (path → checksum)
  local installed_json
  installed_json="$(_marker_load | jq '.installed_checksums')"

  # Track new checksums map (we'll update .ndf.json at the end)
  local new_checksums="{}"

  # ---- pass 1: process each file in the new manifest ----
  echo "$new_paths_json" | jq -c '.[]' | while IFS= read -r entry; do
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
            r|R) cp "$tmp" "$p"; _info "    replaced ${p}." ;;
            b|B) cp "$p" "${p}.local-backup"; cp "$tmp" "$p"; _info "    backed up ${p} → ${p}.local-backup; replaced with framework version." ;;
            *) _info "    skipped ${p}." ;;
          esac
          rm -f "$tmp"
        fi
      fi
    fi

    # Record the new checksum (use the final state of the file on disk for safety).
    if [[ -f "$p" ]]; then
      local final_sha
      final_sha="$(_sha256 "$p")"
      new_checksums="$(echo "$new_checksums" | jq --arg k "$p" --arg v "$final_sha" '. + {($k): $v}')"
    fi
  done

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
  _marker_write "$target_version" "$new_pinned" "$new_checksums"

  _ok "ndf update complete. Now at v${target_version}."
}

# ---------- help ----------

_print_help_init() {
  cat <<EOF
Usage: ndf init [flags]

Scaffold a new ndf project in the current directory.

Flags:
  --token=<framework_pat>            GitHub PAT, read-only on nandu-org/nandu-dev-framework
  --fieldnotes-token=<fieldnotes_pat>  GitHub PAT, write-only on the client's field-notes repo
  --fieldnotes-repo=<owner/repo>     The client's field-notes repo, e.g. nandu-org/field-notes-vera
  --version=<x.y.z>                  Pin to a specific framework version (default: latest tag)

Any provided values are persisted to ~/.config/nandu/config.json (mode 0600). On
subsequent ndf invocations, the file is read silently — no flags needed unless you
want to overwrite a value.

Env vars NDF_GITHUB_TOKEN and NDF_FIELDNOTES_TOKEN override the config file when
set (token-only; fieldnotes_repo has no env-var override).

The /field-note slash command requires both --fieldnotes-token and --fieldnotes-repo.
If either is missing, /field-note prints a clear "not configured" message and stops.
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
  init     Scaffold a new ndf project in the current directory
  update   Update the framework files in the current project
  version  Print the CLI version
  help     Print this help

Run \`ndf <command> --help\` for command-specific help.
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
    update)  cmd_update "$@" ;;
    version) echo "ndf v${NDF_CLI_VERSION}" ;;
    help|-h|--help) _print_help ;;
    *) _print_help; exit 1 ;;
  esac
}

main "$@"
