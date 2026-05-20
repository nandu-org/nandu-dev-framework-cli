package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// cmdSelfUpdate prints channel-aware instructions for updating the `ndf` CLI
// binary itself (as distinct from `ndf update`, which updates the framework
// files in a project).
//
// The subcommand is named `self-update` (not `upgrade`) deliberately. The
// dominant Unix package-manager convention (apt, brew, dnf, zypper, pacman)
// is `update` = refresh catalog / `upgrade` = install newer versions — which
// would put `ndf update` (does the actual file work) closer to `brew upgrade`
// semantics. Picking `ndf upgrade` for the print-instructions command would
// triple-conflict with that convention (brew-`update` and brew-`upgrade` both
// have established meanings; deno/bun/`brew upgrade` all use `upgrade` for
// actual binary replacement, which this command deliberately doesn't do).
// `self-update` (pnpm's pattern) sidesteps every collision: the "self-"
// prefix makes it unambiguous that this command operates on the CLI itself.
//
// We deliberately do NOT replace the binary in place. When ndf was installed
// via a package manager (Homebrew, Scoop), self-replacement would diverge the
// on-disk binary from the package manager's recorded state — the next
// `brew upgrade` / `scoop update` would either no-op against a stale recorded
// version or overwrite the in-place self-update with an older binary. Printing
// the channel-appropriate command keeps package-manager state authoritative.
func cmdSelfUpdate(args []string) {
	for _, a := range args {
		switch a {
		case "-h", "--help":
			printHelpSelfUpdate()
			return
		default:
			die("unknown self-update flag: %s", a)
		}
	}
	printSelfUpdateInstructions(detectInstallChannel())
}

// installChannel names the install pathway we believe produced this binary.
type installChannel string

const (
	channelHomebrew   installChannel = "homebrew"
	channelInstallSh  installChannel = "install.sh"
	channelScoop      installChannel = "scoop"
	channelInstallPs1 installChannel = "install.ps1"
	channelUnknown    installChannel = "unknown"
)

// detectInstallChannel inspects the resolved path of the running binary and
// returns the matching channel, or channelUnknown if no signature fits (e.g.
// manual download to /opt/bin, build from source).
//
// TODO(winget): when adding a channelWinget case, update in lockstep — README
// "Update the CLI" table, CHANGELOG entry, framework METHODOLOGY.md's
// older-CLI fallback enumeration, and the canonical KB §14 channel-list.
// See the ndf-maintainer skill's "Pending — winget submission" Step 0.
func detectInstallChannel() installChannel {
	exe, err := os.Executable()
	if err != nil {
		return channelUnknown
	}
	// Resolve symlinks so Homebrew's `/opt/homebrew/bin/ndf` chains to its
	// Cellar path. Fallback to the unresolved path if resolution fails.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	exe = filepath.ToSlash(exe)

	if runtime.GOOS == "windows" {
		lower := strings.ToLower(exe)
		// Scoop installs the actual binary at `%USERPROFILE%\scoop\apps\ndf\<version>\ndf.exe`
		// and creates a shim on PATH at `\scoop\shims\ndf.exe` (modern kiennq shim — paired
		// with `.shim` config that points at the real binary). The shim launches our binary
		// as a child process; os.Executable() from inside the child returns the actual binary
		// path under `\scoop\apps\ndf\<v>\`, never the shim, so we only need to look for the
		// apps/ndf/ signature regardless of the shim format. (Earlier Scoop versions used
		// `.cmd` wrappers; same property holds.)
		if strings.Contains(lower, "/scoop/apps/ndf/") {
			return channelScoop
		}
		if strings.Contains(lower, "/programs/nandu/") {
			return channelInstallPs1
		}
		return channelUnknown
	}

	// Homebrew (macOS + Linuxbrew): the canonical signal after EvalSymlinks
	// is `.../Cellar/ndf/<version>/bin/ndf`.
	if strings.Contains(exe, "/Cellar/ndf/") {
		return channelHomebrew
	}
	// install.sh always lands the binary at ~/.local/bin/ndf. Resolve symlinks
	// on the home dir too so macOS `/var` → `/private/var` (and similar) don't
	// defeat the comparison.
	if home, err := os.UserHomeDir(); err == nil {
		if resolved, err := filepath.EvalSymlinks(home); err == nil {
			home = resolved
		}
		if exe == filepath.ToSlash(filepath.Join(home, ".local/bin/ndf")) {
			return channelInstallSh
		}
	}
	return channelUnknown
}

// printSelfUpdateInstructions emits the channel-specific upgrade command and
// a short fallback list of other channels.
func printSelfUpdateInstructions(c installChannel) {
	rawErr("ndf v" + CLIVersion + " — self-update instructions")
	rawErr("")
	rawErr("`ndf self-update` prints the command to run; it does not replace")
	rawErr("the binary itself. (Self-replacement would diverge from the package")
	rawErr("manager's recorded state and break the next brew/scoop upgrade.)")
	rawErr("")

	switch c {
	case channelHomebrew:
		rawErr("Detected channel: Homebrew. Update with:")
		rawErr("")
		rawErr("  brew upgrade nandu-org/tap/ndf")
	case channelInstallSh:
		rawErr("Detected channel: install.sh (curl one-liner). Re-run the installer")
		rawErr("to update in place (idempotent):")
		rawErr("")
		rawErr("  curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.sh | bash")
	case channelScoop:
		rawErr("Detected channel: Scoop. Update with:")
		rawErr("")
		rawErr("  scoop update ndf")
	case channelInstallPs1:
		rawErr("Detected channel: install.ps1 (PowerShell one-liner). Re-run the")
		rawErr("installer to update in place (idempotent):")
		rawErr("")
		rawErr("  iwr -useb https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.ps1 | iex")
	default:
		rawErr("Could not auto-detect your install channel. Use the command matching")
		rawErr("how you installed ndf:")
		rawErr("")
		rawErr("  Homebrew (macOS):  brew upgrade nandu-org/tap/ndf")
		rawErr("  Scoop (Windows):   scoop update ndf")
		rawErr("  curl (macOS/Linux):")
		rawErr("    curl -fsSL https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.sh | bash")
		rawErr("  PowerShell (Windows):")
		rawErr("    iwr -useb https://raw.githubusercontent.com/nandu-org/nandu-dev-framework-cli/main/install.ps1 | iex")
		rawErr("  Manual download: https://github.com/nandu-org/nandu-dev-framework-cli/releases")
		rawErr("")
		rawErr("Then verify: ndf version")
		return
	}

	rawErr("")
	rawErr("Then verify: ndf version")
	rawErr("")
	rawErr("Other install channels (if you installed via a different path):")
	rawErr("  https://github.com/nandu-org/nandu-dev-framework-cli/releases")
}
