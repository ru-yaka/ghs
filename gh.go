package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gh "github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/auth"
	ghconfig "github.com/cli/go-gh/v2/pkg/config"
	"gopkg.in/yaml.v3"
)

// GhHostInfo holds information about a GitHub host from gh CLI config.
type GhHostInfo struct {
	Host  string
	User  string
	Token string
}

// ghIsInstalled checks if gh CLI is available.
func ghIsInstalled() bool {
	_, err := gh.Path()
	return err == nil
}

// ghGetToken retrieves the current gh auth token for the default host.
// It checks env vars, config file, and system keyring (via gh auth token).
func ghGetToken() (string, error) {
	token, source := auth.TokenForHost("github.com")
	if token == "" {
		return "", fmt.Errorf("not authenticated with gh CLI (source: %s)", source)
	}
	return token, nil
}

// ghGetUser retrieves the current gh username from config.
func ghGetUser() (string, error) {
	cfg, err := ghconfig.Read(nil)
	if err != nil {
		return "", err
	}
	user, err := cfg.Get([]string{"hosts", "github.com", "user"})
	if err != nil {
		return "", fmt.Errorf("no gh user found in config")
	}
	return user, nil
}

// ghLoginWithToken switches gh auth by piping a token to gh auth login.
func ghLoginWithToken(token string) error {
	ghExe, err := gh.Path()
	if err != nil {
		return fmt.Errorf("gh CLI not found: %w", err)
	}
	cmd := exec.Command(ghExe, "auth", "login", "--with-token", "--hostname", "github.com")
	cmd.Stdin = strings.NewReader(token)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh auth login failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ghAuthRefresh refreshes the auth token for the current gh user.
func ghAuthRefresh() error {
	ghExe, err := gh.Path()
	if err != nil {
		return fmt.Errorf("gh CLI not found: %w", err)
	}
	cmd := exec.Command(ghExe, "auth", "refresh", "--hostname", "github.com")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ghExec runs a gh command and returns combined output.
func ghExec(args ...string) (string, error) {
	stdout, stderr, err := gh.Exec(args...)
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// ghExecWithStdin runs a gh command with custom stdin.
func ghExecWithStdin(args []string, stdin io.Reader) (string, error) {
	ghExe, err := gh.Path()
	if err != nil {
		return "", fmt.Errorf("gh CLI not found: %w", err)
	}
	cmd := exec.Command(ghExe, args...)
	cmd.Stdin = stdin
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ghCreateRepo creates a GitHub repository and sets up the remote.
func ghCreateRepo(name, visibility, remoteName string) (string, error) {
	result, err := ghExec("repo", "create", name, "--"+visibility,
		"--source=.", "--remote="+remoteName, "--push=false")
	if err != nil {
		return "", fmt.Errorf("gh repo create failed: %w", err)
	}
	// Parse URL from output
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://github.com/") || strings.HasPrefix(line, "git@github.com:") {
			return line, nil
		}
	}
	return result, nil
}

// ghGetRepoURL gets the current repo's URL via gh CLI.
func ghGetRepoURL() (string, error) {
	return ghExec("repo", "view", "--json", "url", "-q", ".url")
}

// ghRepoSize returns the repo size in KB via GitHub API.
func ghRepoSize(repoRef string) (int, error) {
	out, err := ghExec("api", "repos/"+repoRef, "-q", ".size")
	if err != nil {
		return 0, err
	}
	var size int
	fmt.Sscanf(out, "%d", &size)
	return size, nil
}

// ghCloneWithProgress clones a repo using gh, streaming progress to terminal.
func ghCloneWithProgress(repoRef, dest string) error {
	ghExe, err := gh.Path()
	if err != nil {
		return fmt.Errorf("gh CLI not found: %w", err)
	}
	cmd := exec.Command(ghExe, "repo", "clone", repoRef, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ghImportHosts reads gh CLI hosts.yml and returns info for all authenticated accounts.
// Supports both old flat format and new multi-account nested format:
//
//	Old format:
//	  github.com:
//	    user: octocat
//	    oauth_token: gho_xxx
//
//	New multi-account format:
//	  github.com:
//	    git_protocol: https
//	    users:
//	      user1:
//	      user2:
//	    user: user1  (active)
func ghImportHosts() ([]GhHostInfo, error) {
	cfgDir := ghconfig.ConfigDir()
	hostsFile := filepath.Join(cfgDir, "hosts.yml")

	data, err := os.ReadFile(hostsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var hosts map[string]interface{}
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", hostsFile, err)
	}

	var result []GhHostInfo

	for host, raw := range hosts {
		if !strings.Contains(host, "github.com") {
			continue
		}
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		// Get active username
		activeUser := ""
		if v, ok := entry["user"].(string); ok {
			activeUser = v
		}

		// Collect all usernames from "users" field (new multi-account format)
		var usernames []string
		if usersRaw, ok := entry["users"]; ok {
			switch v := usersRaw.(type) {
			case map[string]interface{}:
				for u := range v {
					usernames = append(usernames, u)
				}
			case []interface{}:
				for _, u := range v {
					if s, ok := u.(string); ok {
						usernames = append(usernames, s)
					}
				}
			}
		}

		// Fallback: old single-user format (flat "user" + "oauth_token")
		if len(usernames) == 0 && activeUser != "" {
			// Try to get token from oauth_token field directly (old format)
			token := ""
			if v, ok := entry["oauth_token"].(string); ok && v != "" {
				token = v
			}
			if token != "" {
				result = append(result, GhHostInfo{Host: host, User: activeUser, Token: token})
				continue
			}
			// No oauth_token in file, try keyring
			usernames = []string{activeUser}
		}

		// Get active user's token (no switch needed)
		activeToken, _ := auth.TokenForHost(host)

		switched := false
		for _, user := range usernames {
			if user == "" {
				continue
			}
			var token string
			if user == activeUser {
				token = activeToken
			} else {
				// Temporarily switch to get this user's token from keyring
				if _, err := ghExec("auth", "switch", "--user", user); err == nil {
					token, _ = auth.TokenForHost(host)
					switched = true
				}
			}
			if token != "" {
				result = append(result, GhHostInfo{Host: host, User: user, Token: token})
			}
		}

		// Restore original active user if we switched
		if switched && activeUser != "" {
			ghExec("auth", "switch", "--user", activeUser)
		}
	}

	return result, nil
}

// ghGetUserEmail fetches the user's primary verified email from GitHub API.
func ghGetUserEmail() (string, error) {
	out, err := ghExec("api", "user/emails")
	if err != nil {
		return "", fmt.Errorf("cannot fetch emails: %w", err)
	}

	type emailEntry struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	var emails []emailEntry
	if err := json.Unmarshal([]byte(out), &emails); err != nil {
		return "", fmt.Errorf("cannot parse emails: %w", err)
	}

	// Prefer primary verified email
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	// Any verified email
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	// Primary email (even unverified)
	for _, e := range emails {
		if e.Primary {
			return e.Email, nil
		}
	}
	// Any email at all
	if len(emails) > 0 {
		return emails[0].Email, nil
	}

	return "", fmt.Errorf("no email found")
}
