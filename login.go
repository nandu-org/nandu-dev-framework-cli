package main

import (
	"fmt"
	"os"
	"strings"
)

// cmdLogin sets per-developer credentials. Interactive by default with
// hidden input; flags are accepted for non-interactive use (CI).
//
// Behavior preserved from the bash CLI:
//   - If a flag is passed, that value is used directly (no prompt).
//   - If a flag is omitted AND a value already exists in config, the prompt
//     advertises "[press Enter to keep current]" and an empty response keeps it.
//   - If a flag is omitted AND no value exists, the prompt is required for
//     framework_pat and optional for fieldnotes_pat.
//   - Framework PAT is required overall; we error out if both flag and prompt
//     leave it empty.
func cmdLogin(args []string) {
	var cliFrameworkPAT, cliFieldnotesPAT string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--token="):
			cliFrameworkPAT = strings.TrimPrefix(a, "--token=")
		case strings.HasPrefix(a, "--fieldnotes-token="):
			cliFieldnotesPAT = strings.TrimPrefix(a, "--fieldnotes-token=")
		case a == "-h" || a == "--help":
			printHelpLogin()
			return
		default:
			die("unknown login flag: %s", a)
		}
	}

	c, err := loadConfig()
	if err != nil {
		die("%v", err)
	}

	newFramework := cliFrameworkPAT
	newFieldnotes := cliFieldnotesPAT

	if newFramework == "" {
		label := "Framework PAT"
		if c.FrameworkPAT != "" {
			label = label + " [press Enter to keep current]"
		}
		newFramework = promptHidden(label)
		if newFramework == "" {
			newFramework = c.FrameworkPAT
		}
	}

	if newFieldnotes == "" {
		label := "Field-notes PAT"
		if c.FieldnotesPAT != "" {
			label = label + " [press Enter to keep current]"
		} else {
			label = label + " (leave empty if not yet provisioned)"
		}
		newFieldnotes = promptHidden(label)
		if newFieldnotes == "" {
			newFieldnotes = c.FieldnotesPAT
		}
	}

	if newFramework == "" {
		die("framework PAT is required. Get yours from your team's secure credential share.")
	}

	c.FrameworkPAT = newFramework
	c.FieldnotesPAT = newFieldnotes
	if err := saveConfig(c); err != nil {
		die("save config: %v", err)
	}
	ok("tokens saved to %s (mode 0600)", configFile())

	if newFieldnotes == "" {
		warn("field-notes PAT not set. /field-note will not work until you re-run `ndf login` with both tokens.")
	}
}

// cmdConfig dispatches `ndf config <sub>`.
func cmdConfig(args []string) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "show":
		cmdConfigShow()
	case "set":
		cmdConfigSet(args[1:])
	case "", "-h", "--help", "help":
		printHelpConfig()
	default:
		die("unknown config subcommand: %s. Try: ndf config show", sub)
	}
}

// cmdConfigSet handles `ndf config set <key> <value>`.
//
// Currently the only supported key is `fieldnotes-repo`; tokens are still
// set via `ndf login` (the per-developer config has its own ergonomics —
// hidden prompts, env-var overrides — that don't fit a generic setter).
//
// Closes the v2.0.x UX gap where setting a missing fieldnotes_repo on an
// already-initialized project required hand-editing .ndf.json.
func cmdConfigSet(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printHelpConfigSet()
		return
	}
	key := args[0]
	switch key {
	case "fieldnotes-repo":
		if len(args) < 2 {
			die("usage: ndf config set fieldnotes-repo OWNER/REPO")
		}
		val := args[1]
		if err := validateRepoSlug(val); err != nil {
			die("%v", err)
		}
		m, err := loadMarker()
		if err != nil {
			die("%v", err)
		}
		if m == nil {
			die("no %s found in current directory. `ndf config set fieldnotes-repo` only works inside an ndf project — run `ndf init --fieldnotes-repo=%s` from a fresh project, or cd into an existing one.", projectMarker, val)
		}
		m.FieldnotesRepo = val
		if err := writeMarker(m); err != nil {
			die("write marker: %v", err)
		}
		ok("fieldnotes_repo set to %s in %s. Commit this so coworkers pick it up automatically.", val, projectMarker)
	case "framework-pat", "fieldnotes-pat", "token", "fieldnotes-token":
		die("tokens are set via `ndf login`, not `ndf config set`. Run `ndf login` (interactive) or pass --token / --fieldnotes-token flags.")
	default:
		die("unknown config key: %s. Supported: fieldnotes-repo", key)
	}
}

func cmdConfigShow() {
	cfgPath := configFile()
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		fmt.Println("No config at " + cfgPath + ". Run `ndf login` to set up credentials.")
		return
	}
	c, err := loadConfig()
	if err != nil {
		die("%v", err)
	}
	fmt.Println("Per-developer config (" + cfgPath + "):")
	fmt.Println("  framework_pat:  " + maskToken(c.FrameworkPAT))
	fmt.Println("  fieldnotes_pat: " + maskToken(c.FieldnotesPAT))
	if c.FieldnotesRepo != "" {
		fmt.Println("  fieldnotes_repo: " + c.FieldnotesRepo + "  (legacy v1.2.x location; v1.3.0+ reads per-project .ndf.json first)")
	}
	fmt.Println()

	m, _ := loadMarker()
	if m != nil {
		fmt.Println("Per-project marker (./" + projectMarker + "):")
		fmt.Println("  version:         " + valueOr(m.Version, "(unknown)"))
		if m.PinnedVersion == nil {
			fmt.Println("  pinned_version:  null")
		} else {
			fmt.Println("  pinned_version:  " + *m.PinnedVersion)
		}
		if m.FieldnotesRepo != "" {
			fmt.Println("  fieldnotes_repo: " + m.FieldnotesRepo)
		}
	} else {
		fmt.Println("(not currently in an NDF project — no .ndf.json in cwd)")
	}

	fmt.Println()
	fmt.Println("Resolved fieldnotes_repo (for /field-note in this directory):")
	if r := resolveFieldnotesRepo(); r != "" {
		fmt.Println("  " + r)
	} else {
		fmt.Println("  (not configured — /field-note will fail in this directory)")
	}
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
