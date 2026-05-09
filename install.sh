#!/usr/bin/env bash
# install.sh — Bootstrap installer for the ndf CLI on macOS and Linux.
#
# Detects host OS+arch, downloads the matching binary from the latest
# GitHub Release into ~/.local/bin/ndf, makes it executable, and (if needed)
# adds ~/.local/bin to the user's $PATH by appending one line to their
# shell rc file. Idempotent — safe to re-run to upgrade in place.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.sh | bash
#
# Specific version:
#   curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.sh | bash -s -- --version=v2.0.0
#
# Windows users: see install.ps1 for the native Windows installer
# (PowerShell). This script intentionally does not support Windows; the Go
# binary is single-static so there's no need to run this through Git Bash.

set -euo pipefail

readonly NDF_REPO="nandu-org/nandu-dev-framework-cli"
readonly NDF_BIN_DIR="${HOME}/.local/bin"
readonly NDF_BIN="${NDF_BIN_DIR}/ndf"

_die() { echo "install: error: $*" >&2; exit 1; }
_info() { echo "install: $*"; }

# ---------- args ----------
version=""
for arg in "$@"; do
  case "$arg" in
    --version=*) version="${arg#*=}" ;;
    *) _die "unknown flag: $arg" ;;
  esac
done

# ---------- OS+arch detection ----------
case "$(uname -s 2>/dev/null || echo unknown)" in
  Darwin*) os="darwin" ;;
  Linux*)  os="linux" ;;
  MINGW*|MSYS*|CYGWIN*)
    _die "this script is for macOS/Linux. On Windows, use install.ps1:

  iwr -useb https://raw.githubusercontent.com/${NDF_REPO}/main/install.ps1 | iex" ;;
  *)
    _die "unsupported OS: $(uname -s). Manually download from https://github.com/${NDF_REPO}/releases" ;;
esac

case "$(uname -m 2>/dev/null || echo unknown)" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64)  arch="amd64" ;;
  *) _die "unsupported architecture: $(uname -m). Manually download from https://github.com/${NDF_REPO}/releases" ;;
esac

# Linux is amd64-only in v2.0.0; if you're on Linux/arm64, build from source.
if [[ "$os" == "linux" && "$arch" == "arm64" ]]; then
  _die "Linux/arm64 is not yet a release target. Build from source: git clone https://github.com/${NDF_REPO} && cd nandu-dev-framework-cli && go build -o ~/.local/bin/ndf ."
fi

artifact="ndf-${os}-${arch}"
_info "detected ${os}/${arch}"

# ---------- prerequisites ----------
command -v curl >/dev/null 2>&1 || _die "curl is required. Install with your package manager (apt install curl / brew install curl / etc)."

# ---------- resolve version ----------
if [[ -z "$version" ]]; then
  _info "resolving latest release…"
  version="$(curl -fsSL "https://api.github.com/repos/${NDF_REPO}/releases/latest" \
    | grep -oE '"tag_name":\s*"[^"]+"' \
    | head -n 1 \
    | sed -E 's/.*"([^"]+)"/\1/')"
  [[ -n "$version" ]] || _die "could not resolve latest release tag from GitHub API. Check https://github.com/${NDF_REPO}/releases and re-run with --version=vX.Y.Z"
fi
_info "installing ${version}"

# ---------- download binary ----------
download_url="https://github.com/${NDF_REPO}/releases/download/${version}/${artifact}"
checksum_url="https://github.com/${NDF_REPO}/releases/download/${version}/checksums.txt"

mkdir -p "$NDF_BIN_DIR"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

_info "downloading ${artifact}…"
if ! curl -fsSL "$download_url" -o "$tmp"; then
  _die "download failed: ${download_url}"
fi

# ---------- verify checksum ----------
_info "verifying checksum…"
expected_sha=""
checksums="$(curl -fsSL "$checksum_url" 2>/dev/null || echo "")"
if [[ -n "$checksums" ]]; then
  expected_sha="$(echo "$checksums" | awk -v f="$artifact" '$2 == f { print $1 }')"
fi

if [[ -z "$expected_sha" ]]; then
  _info "no checksum available for ${artifact} in checksums.txt; skipping verification."
else
  if command -v sha256sum >/dev/null 2>&1; then
    got_sha="$(sha256sum "$tmp" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    got_sha="$(shasum -a 256 "$tmp" | awk '{print $1}')"
  else
    _info "no sha256 tool found; skipping verification."
    got_sha=""
  fi
  if [[ -n "$got_sha" && "$got_sha" != "$expected_sha" ]]; then
    _die "checksum mismatch! expected ${expected_sha}, got ${got_sha}. Refusing to install."
  fi
  [[ -n "$got_sha" ]] && _info "checksum ok"
fi

# ---------- install ----------
mv "$tmp" "$NDF_BIN"
chmod +x "$NDF_BIN"
trap - EXIT
_info "installed ${NDF_BIN}"

# ---------- PATH setup ----------
_path_has_bin_dir() {
  echo ":$PATH:" | grep -q ":$NDF_BIN_DIR:"
}

if _path_has_bin_dir; then
  _info "${NDF_BIN_DIR} is already on \$PATH; nothing to add."
  ndf_on_path=1
else
  rc_file=""
  case "${SHELL##*/}" in
    zsh)
      rc_file="${HOME}/.zshrc" ;;
    bash)
      # macOS bash convention is .bash_profile; Linux is .bashrc
      if [[ "$os" == "darwin" && -f "${HOME}/.bash_profile" ]]; then
        rc_file="${HOME}/.bash_profile"
      else
        rc_file="${HOME}/.bashrc"
      fi
      ;;
    *)
      _info "warning: shell '${SHELL##*/}' is not zsh or bash; cannot auto-configure \$PATH."
      echo ""
      echo "  Add this to your shell's rc file manually:"
      echo ""
      echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
      echo ""
      _info "ndf is installed at ${NDF_BIN} but won't be on \$PATH until you do that."
      exit 0
      ;;
  esac

  marker="# ndf CLI (added by install.sh)"
  path_line='export PATH="$HOME/.local/bin:$PATH"'
  if [[ -f "$rc_file" ]] && grep -qF "$marker" "$rc_file"; then
    _info "${rc_file} already has the ndf PATH entry; not duplicating."
  else
    {
      echo ""
      echo "$marker"
      echo "$path_line"
    } >> "$rc_file"
    _info "added ndf to \$PATH in ${rc_file}"
  fi
  ndf_on_path=0
fi

# ---------- verify or instruct ----------
echo ""
if [[ "$ndf_on_path" -eq 1 ]] && command -v ndf >/dev/null 2>&1; then
  _info "verifying install:"
  ndf version
  echo ""
  _info "ndf is ready. Next: run \`ndf login\` to set your tokens, then \`cd <project> && ndf init --fieldnotes-repo=<owner/repo>\`."
else
  _info "install complete. To use ndf in this shell, run:"
  echo ""
  echo "  exec \$SHELL -l"
  echo ""
  _info "Or open a new terminal. Then verify with: ndf version"
fi
