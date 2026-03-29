package main

import (
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

// ghImportHosts reads gh CLI hosts.yml directly and returns info for all
// authenticated hosts. gh stores multiple accounts as github.com, github.com-2, etc.
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

	// Parse YAML directly — each top-level key is a hostname entry:
	//   github.com:
	//     user: octocat
	//     oauth_token: gho_xxx
	//   github.com-2:
	//     user: user2
	//     oauth_token: gho_yyy
	var hosts map[string]map[string]string
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", hostsFile, err)
	}

	var result []GhHostInfo
	for host, entry := range hosts {
		if !strings.Contains(host, "github.com") {
			continue
		}

		user := entry["user"]
		if user == "" {
			continue
		}

		// Try token from config file first (PAT tokens stored here)
		token := entry["oauth_token"]

		// If not in file, try keyring/env via go-gh
		if token == "" {
			token, _ = auth.TokenForHost(host)
		}

		// Last resort: call gh CLI directly
		if token == "" {
			if out, err := ghExec("auth", "token", "--hostname", host); err == nil {
				token = strings.TrimSpace(out)
			}
		}

		if token != "" {
			result = append(result, GhHostInfo{
				Host:  host,
				User:  user,
				Token: token,
			})
		}
	}

	return result, nil
}
