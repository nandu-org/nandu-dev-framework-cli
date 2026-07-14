#!/usr/bin/env bash
# verify-show.sh — golden-file check for `ndf config show` rendering.
#
# Builds ./ndf, runs `ndf config show` under each of 5 fixture environments,
# diffs stdout against testdata/config-show/<fixture>.txt. Any diff → exit 1.
#
# Run before tagging any release that touches cmdConfigShow.
set -euo pipefail

cd "$(dirname "$0")/.."

go build -o /tmp/ndf-verify-show . > /tmp/ndf-verify-show.build.log 2>&1 || {
  echo "build failed:" >&2
  cat /tmp/ndf-verify-show.build.log >&2
  exit 1
}

FAIL=0
for fixture in no-config-no-marker config-no-marker no-config-marker config-marker-with-legacy config-old-marker; do
  workdir=$(mktemp -d)
  cfgdir=$(mktemp -d)
  case "$fixture" in
    no-config-no-marker) ;;
    config-no-marker)
      mkdir -p "$cfgdir/nandu"
      cat > "$cfgdir/nandu/config.json" <<EOF
{
  "framework_pat": "ghp_fake1234567890abcdef",
  "fieldnotes_pat": "ghp_fake0987654321zyxwvu"
}
EOF
      ;;
    no-config-marker)
      mkdir -p "$workdir/.ndf/cli"
      cat > "$workdir/.ndf/cli/install.json" <<EOF
{
  "version": "4.2.0",
  "pinned_version": null,
  "installed_checksums": {},
  "fieldnotes_repo": "nandu-org/Example-FieldNotes"
}
EOF
      ;;
    config-marker-with-legacy)
      mkdir -p "$cfgdir/nandu"
      cat > "$cfgdir/nandu/config.json" <<EOF
{
  "framework_pat": "ghp_fake1234567890abcdef",
  "fieldnotes_pat": "ghp_fake0987654321zyxwvu",
  "fieldnotes_repo": "legacy/repo"
}
EOF
      mkdir -p "$workdir/.ndf/cli"
      cat > "$workdir/.ndf/cli/install.json" <<EOF
{
  "version": "4.2.0",
  "pinned_version": null,
  "installed_checksums": {},
  "fieldnotes_repo": "nandu-org/Example-FieldNotes"
}
EOF
      ;;
    config-old-marker)
      # Marker at the pre-v2.5.0 path (.ndf.json) only — exercises config
      # show's dual-path display branch (loadMarkerWithSource == "old"), which
      # must print the OLD resolved location, not the NEW one.
      mkdir -p "$cfgdir/nandu"
      cat > "$cfgdir/nandu/config.json" <<EOF
{
  "framework_pat": "ghp_fake1234567890abcdef",
  "fieldnotes_pat": "ghp_fake0987654321zyxwvu"
}
EOF
      cat > "$workdir/.ndf.json" <<EOF
{
  "version": "4.2.0",
  "pinned_version": null,
  "installed_checksums": {},
  "fieldnotes_repo": "nandu-org/Example-FieldNotes"
}
EOF
      ;;
  esac

  actual=$(XDG_CONFIG_HOME="$cfgdir" CLAUDE_PROJECT_DIR="$workdir" /tmp/ndf-verify-show config show 2>/dev/null || true)
  expected_file="testdata/config-show/$fixture.txt"
  # Normalize the absolute paths that vary per machine/run: the per-developer
  # config path and the resolved per-project marker path (config show now prints
  # the marker's resolved absolute location, honoring CLAUDE_PROJECT_DIR).
  actual_norm=$(echo "$actual" | sed -E "s|$cfgdir/nandu/config.json|<CONFIG_PATH>|g; s|$workdir|<PROJECT_DIR>|g")

  if ! diff -u "$expected_file" <(echo "$actual_norm"); then
    echo "FAIL: fixture $fixture diverged from golden" >&2
    FAIL=1
  else
    echo "PASS: $fixture"
  fi

  rm -rf "$workdir" "$cfgdir"
done

if [[ "$FAIL" != 0 ]]; then
  exit 1
fi
echo "all 5 fixtures match golden output."
