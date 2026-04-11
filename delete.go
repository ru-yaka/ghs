package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// cmdDelete handles: ghs delete <repos|users> <names...> [--yes]
func cmdDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs delete repos <repo...> [--yes]\n       ghs delete users <user...> [--yes]")
	}

	subCmd := args[0]
	rest := args[1:]

	switch subCmd {
	case "repo", "repos":
		return cmdDeleteRepos(rest)
	case "user", "users":
		return cmdDeleteUsers(rest)
	default:
		return fmt.Errorf("usage: ghs delete repos <repo...> [--yes]\n       ghs delete users <user...> [--yes]")
	}
}

// cmdDeleteRepos deletes GitHub repositories.
func cmdDeleteRepos(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs delete repos <repo...> [--yes]")
	}

	// Parse args
	var repos []string
	skipConfirm := false
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			skipConfirm = true
		} else {
			repos = append(repos, a)
		}
	}
	if len(repos) == 0 {
		return fmt.Errorf("usage: ghs delete repos <repo...> [--yes]")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Build a map: account alias → token, for concurrent API calls.
	type accountInfo struct {
		ghUser string
		token  string
	}
	accounts := map[string]accountInfo{}
	for alias, acc := range cfg.Accounts {
		if acc.Token != "" {
			accounts[alias] = accountInfo{ghUser: acc.GhUser, token: acc.Token}
		}
	}

	// resolveRepo maps a user-supplied name to (fullName, token).
	resolveRepo := func(name string) (fullName string, token string, err error) {
		// Already qualified
		if strings.Contains(name, "/") {
			parts := strings.SplitN(name, "/", 2)
			owner := parts[0]
			// Find the account that matches this owner
			for _, acc := range accounts {
				if acc.ghUser == owner {
					return name, acc.token, nil
				}
			}
			return "", "", fmt.Errorf("no account found for owner '%s'", owner)
		}

		// Bare name — match against all accounts' repo lists
		type match struct {
			fullName string
			token    string
		}
		var matches []match

		for _, acc := range accounts {
			repoList, err := fetchUserRepos(acc.token, acc.ghUser)
			if err != nil {
				continue
			}
			for _, r := range repoList {
				if r.Name == name {
					matches = append(matches, match{fullName: r.FullName, token: acc.token})
				}
			}
		}

		switch len(matches) {
		case 0:
			return "", "", fmt.Errorf("repo '%s' not found in any account", name)
		case 1:
			return matches[0].fullName, matches[0].token, nil
		default:
			fmt.Printf("  Repo '%s' found in multiple accounts:\n", name)
			for i, m := range matches {
				fmt.Printf("    %d. %s\n", i+1, m.fullName)
			}
			input, _ := readInput("  Pick one: ")
			idx := 0
			fmt.Sscanf(strings.TrimSpace(input), "%d", &idx)
			if idx < 1 || idx > len(matches) {
				return "", "", fmt.Errorf("invalid selection")
			}
			return matches[idx-1].fullName, matches[idx-1].token, nil
		}
	}

	// Resolve all repos
	type deleteItem struct {
		fullName string
		token    string
	}
	var items []deleteItem
	for _, r := range repos {
		fn, tok, err := resolveRepo(r)
		if err != nil {
			printError("%s: %s", r, err)
			continue
		}
		items = append(items, deleteItem{fullName: fn, token: tok})
	}
	if len(items) == 0 {
		return fmt.Errorf("no repos to delete")
	}

	// Confirm
	if !skipConfirm {
		fmt.Println("Repos to delete:")
		for _, it := range items {
			fmt.Printf("  - %s\n", it.fullName)
		}
		if !confirm("Delete these repositories?") {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Concurrent delete
	refreshCh := make(chan struct{})
	var refreshOnce sync.Once
	var wg sync.WaitGroup

	// tokenFor returns an up-to-date token for a repo's owner.
	tokenFor := func(fullName string) string {
		for _, acc := range accounts {
			if strings.HasPrefix(fullName, acc.ghUser+"/") {
				return acc.token
			}
		}
		return ""
	}

	for _, it := range items {
		wg.Add(1)
		go func(item deleteItem) {
			defer wg.Done()
			err := deleteRepoAPI(item.fullName, item.token)
			if err != nil && strings.Contains(err.Error(), "delete_repo") {
				refreshOnce.Do(func() {
					printInfo("requesting delete_repo permission...")
					if refreshErr := cmdRefreshWithScopes([]string{"delete_repo"}); refreshErr != nil {
						printError("failed to refresh scope: %s", refreshErr)
						close(refreshCh)
						return
					}
					// Reload all tokens
					newCfg, _ := loadConfig()
					for alias, acc := range newCfg.Accounts {
						if acc.Token != "" {
							accounts[alias] = accountInfo{ghUser: acc.GhUser, token: acc.Token}
						}
					}
					close(refreshCh)
				})
				<-refreshCh
				item.token = tokenFor(item.fullName)
				if err = deleteRepoAPI(item.fullName, item.token); err != nil {
					printError("failed to delete %s: %s", item.fullName, err)
					return
				}
			} else if err != nil {
				printError("failed to delete %s: %s", item.fullName, err)
				return
			}
			printSuccess("deleted %s", item.fullName)
		}(it)
	}
	wg.Wait()

	return nil
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
		alias string
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

// deleteRepoAPI calls GitHub API to delete a repository.
func deleteRepoAPI(fullName, token string) error {
	req, err := http.NewRequest("DELETE", "https://api.github.com/repos/"+fullName, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub API error: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 204:
		return nil
	case 403:
		return fmt.Errorf("HTTP 403: must have admin rights and \"delete_repo\" scope")
	case 404:
		return fmt.Errorf("not found: %s", fullName)
	default:
		return fmt.Errorf("GitHub API returned %s", resp.Status)
	}
}
