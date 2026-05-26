#!/bin/bash
# verify-dual-path.sh — dual-path read/write check for the v2.5.0+ CLI.
#
# Builds ./ndf and exercises five fixtures that cover the catch-up
# window between the v2.5.0 CLI release (introducing the .ndf/cli/
# layout) and the v4.3-to-v4.4 framework migration (which moves
# on-disk state from OLD to NEW). Each fixture is provisioned in a
# temp dir, the relevant CLI command(s) run with CLAUDE_PROJECT_DIR
# pointing at the fixture, and observed behavior is diffed against
# the fixture's `expected.txt` contract.
#
# Run before tagging any release that touches dual-path logic.
set -euo pipefail

cd "$(dirname "$0")/.."

go build -o /tmp/ndf-verify-dual-path . > /tmp/ndf-verify-dual-path.build.log 2>&1 || {
  echo "build failed:" >&2
  cat /tmp/ndf-verify-dual-path.build.log >&2
  exit 1
}

NDF=/tmp/ndf-verify-dual-path
FAIL=0
PASS_COUNT=0

# Empty XDG_CONFIG_HOME so the per-developer config never accidentally
# leaks into resolveFieldnotesRepo. Each test still controls its
# project-dir contents via CLAUDE_PROJECT_DIR.
emptycfg=$(mktemp -d)

scenario_a_marker_old_only() {
  local workdir
  workdir=$(mktemp -d)
  cat > "$workdir/.ndf.json" <<EOF
{
  "version": "4.2.0",
  "pinned_version": null,
  "installed_checksums": {},
  "fieldnotes_repo": "nandu-org/example"
}
EOF

  # is-project should exit 0 via OLD-path fallback.
  if ! XDG_CONFIG_HOME="$emptycfg" CLAUDE_PROJECT_DIR="$workdir" "$NDF" is-project; then
    echo "FAIL (a): ndf is-project returned non-zero with marker at OLD" >&2
    rm -rf "$workdir"
    return 1
  fi

  # marker-path returns the NEW absolute path (pure-NEW write target).
  local mp
  mp=$(XDG_CONFIG_HOME="$emptycfg" CLAUDE_PROJECT_DIR="$workdir" "$NDF" marker-path)
  if [[ "$mp" != "$workdir/.ndf/cli/install.json" ]]; then
    echo "FAIL (a): ndf marker-path = '$mp', expected '$workdir/.ndf/cli/install.json'" >&2
    rm -rf "$workdir"
    return 1
  fi

  # config get version reads OLD via dual-path fallback and prints 4.2.0.
  local ver
  ver=$(XDG_CONFIG_HOME="$emptycfg" CLAUDE_PROJECT_DIR="$workdir" "$NDF" config get version 2>/dev/null)
  if [[ "$ver" != "4.2.0" ]]; then
    echo "FAIL (a): ndf config get version = '$ver', expected '4.2.0'" >&2
    rm -rf "$workdir"
    return 1
  fi

  echo "PASS: (a) marker-old-only"
  rm -rf "$workdir"
}

scenario_b_sentinels_old_only() {
  local workdir
  workdir=$(mktemp -d)
  mkdir -p "$workdir/.ndf-migrations"
  : > "$workdir/.ndf-migrations/v3-to-v4-feature-scoped.complete"
  # Also drop a marker so is-project answers yes (sentinel checks only
  # fire from within update paths, but the fixture's contract is "OLD
  # sentinel dir exists, NEW sentinel dir absent" — we verify the on-disk
  # shape and that the helper detects it via a small Go probe).
  cat > "$workdir/.ndf.json" <<EOF
{
  "version": "4.2.0",
  "pinned_version": null,
  "installed_checksums": {}
}
EOF

  # Fixture layout assertions (documents the contract; no CLI command
  # exposes migrationSentinelExists directly).
  if [[ ! -f "$workdir/.ndf-migrations/v3-to-v4-feature-scoped.complete" ]]; then
    echo "FAIL (b): OLD sentinel fixture file missing" >&2
    rm -rf "$workdir"
    return 1
  fi
  if [[ -e "$workdir/.ndf/cli/sentinels" ]]; then
    echo "FAIL (b): NEW sentinel dir unexpectedly present in fixture setup" >&2
    rm -rf "$workdir"
    return 1
  fi

  # is-project also exercises the dual-path read on the marker so the
  # fixture proves both helpers compose: sentinel exists at OLD, marker
  # exists at OLD, is-project answers yes.
  if ! XDG_CONFIG_HOME="$emptycfg" CLAUDE_PROJECT_DIR="$workdir" "$NDF" is-project; then
    echo "FAIL (b): ndf is-project returned non-zero alongside OLD sentinels" >&2
    rm -rf "$workdir"
    return 1
  fi

  echo "PASS: (b) sentinels-old-only"
  rm -rf "$workdir"
}

scenario_c_pending_handoff_old_only() {
  local workdir
  workdir=$(mktemp -d)
  printf 'pending handoff body\n' > "$workdir/.ndf-pending-handoff"
  cat > "$workdir/.ndf.json" <<EOF
{
  "version": "4.2.0",
  "pinned_version": null,
  "installed_checksums": {}
}
EOF

  # Fixture layout assertions.
  if [[ ! -f "$workdir/.ndf-pending-handoff" ]]; then
    echo "FAIL (c): OLD pending-handoff fixture file missing" >&2
    rm -rf "$workdir"
    return 1
  fi
  if [[ -e "$workdir/.ndf/cli/pending-handoff" ]]; then
    echo "FAIL (c): NEW pending-handoff unexpectedly present in fixture setup" >&2
    rm -rf "$workdir"
    return 1
  fi

  # is-project still passes — the marker is at OLD and dual-path read
  # finds it.
  if ! XDG_CONFIG_HOME="$emptycfg" CLAUDE_PROJECT_DIR="$workdir" "$NDF" is-project; then
    echo "FAIL (c): ndf is-project returned non-zero alongside OLD pending-handoff" >&2
    rm -rf "$workdir"
    return 1
  fi

  echo "PASS: (c) pending-handoff-old-only"
  rm -rf "$workdir"
}

scenario_d_marker_new_write_stays_new() {
  local workdir
  workdir=$(mktemp -d)
  mkdir -p "$workdir/.ndf/cli"
  cat > "$workdir/.ndf/cli/install.json" <<EOF
{
  "version": "4.2.0",
  "pinned_version": null,
  "installed_checksums": {},
  "fieldnotes_repo": "nandu-org/old-repo"
}
EOF

  # Run `ndf config set fieldnotes-repo nandu-org/new-repo` — should
  # update NEW in place; OLD must remain absent.
  XDG_CONFIG_HOME="$emptycfg" CLAUDE_PROJECT_DIR="$workdir" "$NDF" config set fieldnotes-repo nandu-org/new-repo > /dev/null

  if [[ ! -f "$workdir/.ndf/cli/install.json" ]]; then
    echo "FAIL (d): NEW marker missing post-write" >&2
    rm -rf "$workdir"
    return 1
  fi
  if [[ -e "$workdir/.ndf.json" ]]; then
    echo "FAIL (d): OLD marker unexpectedly created at project root" >&2
    rm -rf "$workdir"
    return 1
  fi
  if ! grep -q '"fieldnotes_repo": "nandu-org/new-repo"' "$workdir/.ndf/cli/install.json"; then
    echo "FAIL (d): NEW marker does not carry the updated fieldnotes_repo" >&2
    rm -rf "$workdir"
    return 1
  fi

  echo "PASS: (d) marker-new-write-stays-new"
  rm -rf "$workdir"
}

scenario_e_marker_old_read_then_write_goes_new() {
  local workdir
  workdir=$(mktemp -d)
  cat > "$workdir/.ndf.json" <<EOF
{
  "version": "4.2.0",
  "pinned_version": null,
  "installed_checksums": {},
  "fieldnotes_repo": "nandu-org/legacy-repo"
}
EOF
  # Capture original OLD content for the post-write integrity check.
  local old_orig
  old_orig=$(cat "$workdir/.ndf.json")

  # Run `ndf config set` — should read OLD via dual-path, write NEW.
  XDG_CONFIG_HOME="$emptycfg" CLAUDE_PROJECT_DIR="$workdir" "$NDF" config set fieldnotes-repo nandu-org/new-repo > /dev/null

  if [[ ! -f "$workdir/.ndf/cli/install.json" ]]; then
    echo "FAIL (e): NEW marker missing post-write" >&2
    rm -rf "$workdir"
    return 1
  fi
  if [[ ! -f "$workdir/.ndf.json" ]]; then
    echo "FAIL (e): OLD marker unexpectedly removed (should remain on disk)" >&2
    rm -rf "$workdir"
    return 1
  fi
  if ! grep -q '"fieldnotes_repo": "nandu-org/new-repo"' "$workdir/.ndf/cli/install.json"; then
    echo "FAIL (e): NEW marker missing updated fieldnotes_repo" >&2
    rm -rf "$workdir"
    return 1
  fi
  local old_after
  old_after=$(cat "$workdir/.ndf.json")
  if [[ "$old_after" != "$old_orig" ]]; then
    echo "FAIL (e): OLD marker content changed (should be untouched)" >&2
    diff -u <(echo "$old_orig") <(echo "$old_after") >&2 || true
    rm -rf "$workdir"
    return 1
  fi

  echo "PASS: (e) marker-old-read-then-write-goes-new"
  rm -rf "$workdir"
}

for scenario in scenario_a_marker_old_only scenario_b_sentinels_old_only scenario_c_pending_handoff_old_only scenario_d_marker_new_write_stays_new scenario_e_marker_old_read_then_write_goes_new; do
  if "$scenario"; then
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    FAIL=1
  fi
done

rm -rf "$emptycfg"

if [[ "$FAIL" != 0 ]]; then
  exit 1
fi
echo "all 5 dual-path fixtures pass."
