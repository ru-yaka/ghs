package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const gistDesc = "ghs config sync"

// syncGistID returns the stored gist ID, or empty if not set.
func syncGistID() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(dir + "/sync-gist-id")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(data)), nil
}

func saveSyncGistID(id string) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	return os.WriteFile(dir+"/sync-gist-id", []byte(id+"\n"), 0600)
}

// cmdSync handles: ghs sync push|pull [alias]
func cmdSync(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs sync push|pull [alias]")
	}

	if !ghIsInstalled() {
		return fmt.Errorf("gh CLI is required for sync")
	}

	action := args[0]
	alias := ""
	if len(args) >= 2 {
		alias = args[1]
	}

	// Resolve which gh account to use for gist operations
	restore, err := ensureGhAuth(alias)
	if err != nil {
		return err
	}
	defer restore()

	switch action {
	case "push":
		return syncPush()
	case "pull":
		return syncPull()
	default:
		return fmt.Errorf("usage: ghs sync push|pull [alias]")
	}
}

// ensureGhAuth ensures the correct gh account is active for gist operations.
// Returns a restore function to switch back to the original account.
func ensureGhAuth(alias string) (func(), error) {
	origUser, _ := ghGetUser()

	// If alias specified, switch to that account
	if alias != "" {
		acc, err := getAccount(alias)
		if err != nil {
			return nil, err
		}
		if acc.Token == "" {
			return nil, fmt.Errorf("account '%s' has no token", alias)
		}
		if err := ghLoginWithToken(acc.Token); err != nil {
			return nil, fmt.Errorf("cannot switch gh auth to '%s': %w", alias, err)
		}
		newUser, _ := ghGetUser()
		printInfo("using gh account: %s", newUser)
		return func() {
			if origUser != "" && origUser != newUser {
				// Try to restore original account
				origAcc, err := findAccountByGhUser(origUser)
				if err == nil && origAcc.Token != "" {
					ghLoginWithToken(origAcc.Token)
				}
			}
		}, nil
	}

	// No alias specified: use current gh auth, or auto-switch if only 1 account has token
	if origUser != "" {
		return func() {}, nil
	}

	// Not authenticated — try to find an account with a token
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	var withToken []string
	for a, acc := range cfg.Accounts {
		if acc.Token != "" {
			withToken = append(withToken, a)
		}
	}

	switch len(withToken) {
	case 0:
		return nil, fmt.Errorf("no account with token found. Run 'ghs import' or 'ghs add <alias> -t <token>'")
	case 1:
		acc := cfg.Accounts[withToken[0]]
		if err := ghLoginWithToken(acc.Token); err != nil {
			return nil, fmt.Errorf("cannot authenticate as '%s': %w", withToken[0], err)
		}
		newUser, _ := ghGetUser()
		printInfo("using gh account: %s", newUser)
		return func() {}, nil
	default:
		return nil, fmt.Errorf("multiple accounts with tokens (%s). Specify one: ghs sync push|pull <alias>", strings.Join(withToken, ", "))
	}
}

// findAccountByGhUser finds an account by its gh username.
func findAccountByGhUser(ghUser string) (*Account, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	for _, acc := range cfg.Accounts {
		if acc.GhUser == ghUser {
			acc := acc
			return &acc, nil
		}
	}
	return nil, fmt.Errorf("no account with gh user '%s'", ghUser)
}

func syncPush() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}

	gistID, _ := syncGistID()

	if gistID != "" {
		printInfo("updating gist %s...", gistID[:8])
		_, err := ghExecWithStdin(
			[]string{"gist", "edit", gistID, "-f", "ghs-config.json", "-"},
			strings.NewReader(string(data)),
		)
		if err != nil {
			printInfo("gist edit failed, creating new one...")
			gistID = ""
		} else {
			printSuccess("config pushed to gist (%d accounts)", len(cfg.Accounts))
			return nil
		}
	}

	printInfo("creating private gist...")
	result, err := ghExecWithStdin(
		[]string{"gist", "create", "-d", gistDesc, "-f", "ghs-config.json", "-"},
		strings.NewReader(string(data)),
	)
	if err != nil {
		return fmt.Errorf("gh gist create failed: %w", err)
	}

	gistID = extractGistID(result)
	if gistID == "" {
		return fmt.Errorf("cannot parse gist ID from: %s", result)
	}
	if err := saveSyncGistID(gistID); err != nil {
		printError("cannot save gist ID: %s", err)
	}

	printSuccess("config pushed to gist %s (%d accounts)", gistID[:8], len(cfg.Accounts))
	return nil
}

func syncPull() error {
	gistID, err := syncGistID()
	if err != nil || gistID == "" {
		return fmt.Errorf("no sync gist found. Run 'ghs sync push' first")
	}

	result, err := ghExec("gist", "view", gistID, "-f", "ghs-config.json", "--raw")
	if err != nil {
		return fmt.Errorf("cannot read gist: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal([]byte(result), &cfg); err != nil {
		return fmt.Errorf("invalid config data: %w", err)
	}
	if cfg.Accounts == nil {
		cfg.Accounts = make(map[string]Account)
	}

	if err := saveConfig(&cfg); err != nil {
		return err
	}

	printSuccess("synced %d account(s) from gist", len(cfg.Accounts))
	return nil
}

func extractGistID(urlOrID string) string {
	parts := strings.Split(urlOrID, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		p := strings.TrimSpace(parts[i])
		if p != "" && p != "gist.github.com" && len(p) >= 20 {
			return p
		}
	}
	trimmed := strings.TrimSpace(urlOrID)
	if len(trimmed) >= 20 {
		return trimmed
	}
	return ""
}
