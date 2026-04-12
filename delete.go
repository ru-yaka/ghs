package main

import (
	"fmt"
)

// cmdDelete handles: ghs delete users <names...> [--yes]
func cmdDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs delete users <user...> [--yes]")
	}

	subCmd := args[0]
	rest := args[1:]

	switch subCmd {
	case "user", "users":
		return cmdDeleteUsers(rest)
	default:
		return fmt.Errorf("usage: ghs delete users <user...> [--yes]")
	}
}

// cmdDeleteUsers removes saved accounts (not GitHub users, just local config).
func cmdDeleteUsers(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs delete users <user...> [--yes]")
	}

	// Parse args
	var users []string
	skipConfirm := false
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			skipConfirm = true
		} else {
			users = append(users, a)
		}
	}
	if len(users) == 0 {
		return fmt.Errorf("usage: ghs delete users <user...> [--yes]")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Resolve each user (supports fragment matching)
	type resolvedUser struct {
		alias  string
		ghUser string
	}
	var resolved []resolvedUser
	for _, u := range users {
		resolvedAlias, err := resolveAlias(u)
		if err != nil {
			printError("%s: %s", u, err)
			continue
		}
		acc := cfg.Accounts[resolvedAlias]
		resolved = append(resolved, resolvedUser{alias: resolvedAlias, ghUser: acc.GhUser})
	}
	if len(resolved) == 0 {
		return fmt.Errorf("no users to delete")
	}

	// Confirm
	if !skipConfirm {
		fmt.Println("Users to delete from ghs config:")
		for _, u := range resolved {
			fmt.Printf("  - %s", u.alias)
			if u.ghUser != "" && u.ghUser != u.alias {
				fmt.Printf(" (gh: %s)", u.ghUser)
			}
			fmt.Println()
		}
		if !confirm("Remove these accounts?") {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Delete
	for _, u := range resolved {
		delete(cfg.Accounts, u.alias)
		printSuccess("removed %s", u.alias)
	}

	return saveConfig(cfg)
}
