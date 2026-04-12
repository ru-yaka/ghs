package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Commit struct {
	Hash        string
	AuthorName  string
	AuthorEmail string
	Subject     string
}

func gitExec(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "FILTER_BRANCH_SQUELCH_WARNING=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func gitConfigSet(key, value string) error {
	_, err := gitExec("config", "--global", key, value)
	return err
}

func gitConfigGet(key string) (string, error) {
	out, err := gitExec("config", "--get", key)
	if err != nil {
		return "", fmt.Errorf("git config %s not set", key)
	}
	return out, nil
}

func getCurrentUser() (name, email string, err error) {
	name, err = gitConfigGet("user.name")
	if err != nil {
		return "", "", fmt.Errorf("cannot get git user.name: %w", err)
	}
	email, err = gitConfigGet("user.email")
	if err != nil {
		return "", "", fmt.Errorf("cannot get git user.email: %w", err)
	}
	return name, email, nil
}

func getCurrentBranch() (string, error) {
	// symbolic-ref works even in empty repos (no commits yet)
	out, err := gitExec("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("cannot get current branch: %w", err)
	}
	if out == "" {
		return "", fmt.Errorf("detached HEAD state")
	}
	return out, nil
}

func hasUpstream() bool {
	_, err := gitExec("rev-parse", "--abbrev-ref", "@{u}")
	return err == nil
}

func getCommits(base string) ([]Commit, error) {
	args := []string{"log", "--format=%H|%an|%ae|%s"}
	if base != "" {
		args = append(args, base)
	}
	out, err := gitExec(args...)
	if err != nil {
		return nil, fmt.Errorf("cannot get commits: %w", err)
	}
	if out == "" {
		return nil, nil
	}

	lines := strings.Split(out, "\n")
	commits := make([]Commit, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, Commit{
			Hash:        parts[0],
			AuthorName:  parts[1],
			AuthorEmail: parts[2],
			Subject:     parts[3],
		})
	}
	return commits, nil
}

func softReset(target string) error {
	_, err := gitExec("reset", "--soft", target)
	if err != nil {
		return fmt.Errorf("git reset failed: %w", err)
	}
	return nil
}

func resetToEmpty() error {
	_, err := gitExec("update-ref", "-d", "HEAD")
	if err != nil {
		return fmt.Errorf("git update-ref failed: %w", err)
	}
	return stageAll()
}

func stageAll() error {
	_, err := gitExec("add", "--all")
	return err
}

func push(remote, branch string, setUpstream bool) error {
	args := []string{"push"}
	if setUpstream {
		args = append(args, "-u")
	}
	args = append(args, remote, branch)
	_, err := gitExec(args...)
	if err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	return nil
}

func hasRemote(name string) bool {
	_, err := gitExec("remote", "get-url", name)
	return err == nil
}

func getRemoteURL(name string) (string, error) {
	out, err := gitExec("remote", "get-url", name)
	if err != nil {
		return "", fmt.Errorf("remote '%s' not found", name)
	}
	return out, nil
}

func getRepoName() (string, error) {
	if hasRemote("origin") {
		url, err := getRemoteURL("origin")
		if err == nil {
			name := extractRepoName(url)
			if name != "" {
				return name, nil
			}
		}
	}
	out, err := gitExec("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("cannot determine repo name")
	}
	base := filepath.Base(strings.TrimSpace(out))
	return strings.TrimSuffix(base, ".git"), nil
}

func getCommitCount() (int, error) {
	out, err := gitExec("rev-list", "--count", "HEAD")
	if err != nil {
		return 0, err
	}
	var count int
	fmt.Sscanf(out, "%d", &count)
	return count, nil
}

func isGitRepo() bool {
	_, err := gitExec("rev-parse", "--git-dir")
	return err == nil
}

// gitCloneWithProgress clones a repo with --progress, streaming output to terminal.
func gitCloneWithProgress(url, dest string) error {
	cmd := exec.Command("git", "clone", "--progress", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getCommitCountOrZero() int {
	count, err := getCommitCount()
	if err != nil {
		return 0
	}
	return count
}

// ensureFilterRepo ensures git-filter-repo is available.
// Checks PATH, then ~/.ghs/git-filter-repo, downloads standalone script if needed.
func ensureFilterRepo() (string, error) {
	// Check PATH first
	if path, err := exec.LookPath("git-filter-repo"); err == nil {
		return path, nil
	}

	// Check ~/.ghs/git-filter-repo
	dir, _ := configDir()
	localPath := filepath.Join(dir, "git-filter-repo")
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		return localPath, nil
	}

	// Download standalone script
	printInfo("downloading git-filter-repo...")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("cannot create config dir: %w", err)
	}

	resp, err := http.Get("https://raw.githubusercontent.com/newren/git-filter-repo/main/git-filter-repo")
	if err != nil {
		return "", fmt.Errorf("cannot download git-filter-repo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	f, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("cannot write git-filter-repo: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(localPath)
		return "", fmt.Errorf("download incomplete: %w", err)
	}

	printSuccess("downloaded git-filter-repo")
	return localPath, nil
}

// filterRepoRewrite rewrites commit authors using git-filter-repo --mailmap.
func filterRepoRewrite(mailmapPath string) error {
	filterRepo, err := ensureFilterRepo()
	if err != nil {
		return err
	}

	// Run git-filter-repo with python3 explicitly (more reliable than shebang)
	cmd := exec.Command("python3", filterRepo, "--mailmap", mailmapPath, "--force")
	out, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(out))
		if errMsg == "" {
			errMsg = "git-filter-repo failed (ensure Python 3 is installed)"
		}
		return fmt.Errorf("git-filter-repo failed: %s", errMsg)
	}
	return nil
}
