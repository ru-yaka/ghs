package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// repoInfo holds basic repo info from GitHub API.
type repoInfo struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Private     bool   `json:"private"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
}

// cmdRepos handles: ghs repos [alias]
func cmdRepos(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Accounts) == 0 {
		return fmt.Errorf("no accounts saved. Run 'ghs add' or 'ghs import' first")
	}

	if len(args) > 0 {
		// List repos for specific account
		alias := args[0]
		acc, err := getAccount(alias)
		if err != nil {
			return err
		}
		return listReposForAccount(alias, acc)
	}

	// List repos for all accounts
	for alias, acc := range cfg.Accounts {
		fmt.Printf("\n%s:\n", alias)
		if err := listReposForAccount(alias, &acc); err != nil {
			printError("%s", err)
		}
	}
	return nil
}

// listReposForAccount fetches and displays repos for one account.
func listReposForAccount(alias string, acc *Account) error {
	if acc.Token == "" {
		return fmt.Errorf("no token saved for '%s'", alias)
	}

	// Determine GitHub username
	username := acc.GhUser
	if username == "" {
		// Try to get username from token
		user, err := ghUserFromToken(acc.Token)
		if err != nil {
			return fmt.Errorf("cannot determine GitHub user: %w", err)
		}
		username = user
	}

	// Fetch repos
	repos, err := fetchUserRepos(acc.Token, username)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		fmt.Println("  (no repos)")
		return nil
	}

	for _, r := range repos {
		if r.Private {
			fmt.Printf("  %s [\x1b[90mprivate\x1b[0m]\n", r.Name)
		} else {
			fmt.Printf("  %s [\x1b[32mpublic\x1b[0m]\n", r.Name)
		}
	}
	fmt.Printf("  (%d repo%s)\n", len(repos), plural(len(repos)))
	return nil
}

// fetchUserRepos fetches repos for a GitHub user using their token.
func fetchUserRepos(token, username string) ([]repoInfo, error) {
	// Build URL with query params
	u, _ := url.Parse("https://api.github.com/user/repos")
	q := u.Query()
	q.Set("affiliation", "owner")
	q.Set("sort", "updated")
	q.Set("per_page", "100")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var repos []repoInfo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("cannot parse response: %w", err)
	}

	return repos, nil
}

// ghUserFromToken gets the GitHub username from a token.
func ghUserFromToken(token string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned %s", resp.Status)
	}

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}
	return user.Login, nil
}

// plural returns "s" if n != 1.
func plural(n int) string {
	if n != 1 {
		return "s"
	}
	return ""
}
