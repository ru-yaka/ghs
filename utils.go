package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var version = "dev"

func confirm(prompt string) bool {
	fmt.Printf("%s (y/N): ", prompt)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func extractRepoName(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func printSuccess(format string, args ...interface{}) {
	fmt.Printf("  ✓ %s\n", fmt.Sprintf(format, args...))
}

func printError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "  ✗ %s\n", fmt.Sprintf(format, args...))
}

func printInfo(format string, args ...interface{}) {
	fmt.Printf("  → %s\n", fmt.Sprintf(format, args...))
}

func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// formatSize formats KB to human-readable string.
func formatSize(kb int) string {
	if kb < 1024 {
		return fmt.Sprintf("%d KB", kb)
	}
	if gb := kb / 1024; gb < 1024 {
		return fmt.Sprintf("%d MB", gb)
	}
	return fmt.Sprintf("%.1f GB", float64(kb)/1024.0/1024.0)
}

func shortRef(ref string) string {
	if strings.HasPrefix(ref, "@{u}") {
		return "upstream"
	}
	return shortHash(ref)
}

func printUsage() {
	fmt.Printf(`ghs - Git & GitHub account switcher v%s

Usage:
  ghs add <alias> [flags]       Add account (auto-imports gh token)
  ghs import [--force]          Import accounts from gh CLI
  ghs remove <alias>            Remove saved account
  ghs use <alias>               Switch git/gh to account
  ghs list                      List saved accounts
  ghs whoami                    Show current git/gh identity
  ghs fix <repo> [alias]        Rewrite commits + switch + force push
                                 repo: URL, owner/repo, or "." for current dir
  ghs sync push|pull [alias]   Sync accounts via encrypted private Gist
  ghs sync key                  Show sync encryption key
  ghs push [--public]           Push (auto-create repo if needed)
  ghs update [version]          Self-upgrade to latest (or specific version)

Flags for 'add':
  -e, --email <email>          Author email (default: current git config)
  -t, --token <token>          GitHub token (default: import from gh CLI)

Flags for 'push':
  --public                     Create public repo (default: private)

Examples:
  ghs add work -e jane@company.com
  ghs import                    # import all gh CLI accounts
  ghs use work                  # switch to work account
  ghs fix .                     # fix current repo with default account
  ghs fix . work                # fix current repo with work account
  ghs fix owner/repo            # clone + fix repo with default account
  ghs fix owner/repo work       # clone + fix repo with work account
`, version)
}

// extractAlias separates the first non-flag argument from flags.
// Supports both: `ghs add work -n Name` and `ghs add -n Name work`.
func extractAlias(args []string) (string, []string) {
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			alias := arg
			rest := append(args[:i:i], args[i+1:]...)
			return alias, rest
		}
	}
	return "", args
}
