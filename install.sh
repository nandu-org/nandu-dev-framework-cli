#!/usr/bin/env bash
# install.sh — Bootstrap installer for the ndf CLI.
#
# Downloads ndf to ~/.local/bin/ndf, makes it executable, and (if needed)
# adds ~/.local/bin to the user's $PATH by appending one line to their
# shell rc file. Idempotent — safe to re-run to upgrade the CLI in place.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.sh | bash

set -euo pipefail

readonly NDF_REPO="nandu-org/nandu-dev-framework-cli"
readonly NDF_RAW_URL="https://raw.githubusercontent.com/${NDF_REPO}/main/ndf.sh"
readonly NDF_BIN_DIR="${HOME}/.local/bin"
readonly NDF_BIN="${NDF_BIN_DIR}/ndf"

_die() { echo "install: error: $*" >&2; exit 1; }
_info() { echo "install: $*"; }

# ---------- OS check ----------
case "$(uname -s 2>/dev/null || echo unknown)" in
  Linux*|Darwin*) ;;
  MINGW*|MSYS*|CYGWIN*)
    _die "ndf requires bash on macOS/Linux/WSL. Native Windows shells are not supported (use WSL — Claude Code itself runs there)." ;;
  *)
    _info "warning: untested OS '$(uname -s)'; proceeding anyway" ;;
esac

# ---------- prerequisites ----------
command -v curl >/dev/null 2>&1 || _die "curl is required (install with: apt install curl / brew install curl / etc.)"

# ---------- download ndf.sh ----------
_info "downloading ndf from ${NDF_REPO}…"
mkdir -p "$NDF_BIN_DIR"
if ! curl -fsSL "$NDF_RAW_URL" -o "$NDF_BIN"; then
  _die "download failed (check network connection and that ${NDF_RAW_URL} is reachable)"
fi
chmod +x "$NDF_BIN"
_info "installed ${NDF_BIN}"

# ---------- PATH setup ----------
_path_has_bin_dir() {
  echo "$PATH" | tr ':' '\n' | grep -qx "$NDF_BIN_DIR"
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
      if [[ "$(uname -s)" == "Darwin" && -f "${HOME}/.bash_profile" ]]; then
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
  _info "ndf is ready. Next: cd into a project and run \`ndf init --token=<your_pat>\`"
else
  _info "install complete. To use ndf in this shell, run:"
  echo ""
  echo "  exec \$SHELL -l"
  echo ""
  _info "Or open a new terminal. Then verify with: ndf version"
fi
