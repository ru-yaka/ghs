package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const version = "0.9.1"

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
  ghs undo [flags]              Undo commits by wrong author
  ghs push [flags]              Push (auto-create repo if needed)
  ghs fix <alias> [flags]       Undo + switch in one step

Flags for 'add':
  -n, --name <name>            Author name (default: current git config)
  -e, --email <email>          Author email (default: current git config)
  -t, --token <token>          GitHub token (default: import from gh CLI)

Flags for 'push':
  --public                     Create public repo (default: private)
  -r, --remote <name>          Remote name (default: origin)

Flags for 'undo' and 'fix':
  --all                        All commits (not just unpushed)
  --last <n>                   Only last N commits
  -y, --yes                    Skip confirmation

Examples:
  ghs add work -n "Jane" -e jane@company.com
  ghs add personal              # imports current gh token automatically
  ghs import                    # import all gh CLI accounts
  ghs fix work -y               # undo wrong commits + switch to work
  ghs push                      # push, create repo if needed
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
