package main

import (
	"bufio"
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

// loadSyncKey reads the stored encryption key, returns empty if not set.
func loadSyncKey() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(dir + "/sync-key")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(data)), nil
}

func saveSyncKey(key string) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	return os.WriteFile(dir+"/sync-key", []byte(key+"\n"), 0600)
}

// cmdSync handles: ghs sync push|pull|key [alias]
func cmdSync(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs sync push|pull|key [alias]")
	}

	if !ghIsInstalled() {
		return fmt.Errorf("gh CLI is required for sync")
	}

	action := args[0]
	alias := ""
	if len(args) >= 2 && action != "key" {
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
	case "key":
		return showSyncKey()
	default:
		return fmt.Errorf("usage: ghs sync push|pull|key [alias]")
	}
}

// showSyncKey displays the current sync encryption key.
func showSyncKey() error {
	key, err := loadSyncKey()
	if err != nil {
		return err
	}
	if key == "" {
		return fmt.Errorf("no sync key. Run 'ghs sync push' first")
	}
	fmt.Printf("  Sync key: %s\n", key)
	printInfo("use this key on another machine: ghs sync pull")
	return nil
}

// ensureGhAuth ensures the correct gh account is active for gist operations.
// Returns a restore function to switch back to the original account.
func ensureGhAuth(alias string) (func(), error) {
	origUser, _ := ghGetUser()

	// If alias specified, switch to that account
	if alias != "" {
		alias, err := resolveAlias(alias)
		if err != nil {
			return nil, err
		}
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
		// No accounts with token — prompt for a temporary token
		printInfo("no account with token found")
		token, err := readInput("GitHub token (for gist access only): ")
		if err != nil || strings.TrimSpace(token) == "" {
			return nil, fmt.Errorf("token required for sync. Run 'ghs import' or 'ghs add <alias> -t <token>'")
		}
		token = strings.TrimSpace(token)
		if err := ghLoginWithToken(token); err != nil {
			return nil, fmt.Errorf("token authentication failed: %w", err)
		}
		newUser, _ := ghGetUser()
		printInfo("authenticated as: %s", newUser)
		return func() {}, nil
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

// readInput prompts and reads a line from stdin.
func readInput(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", fmt.Errorf("cannot read input: %w", err)
	}
	return strings.TrimSpace(line), nil
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

	// Get or create encryption key
	key, err := loadSyncKey()
	if err != nil {
		return err
	}

	if key == "" {
		// First time: let user specify or auto-generate
		key, err = readInput("Encryption key (leave empty to auto-generate): ")
		if err != nil {
			return err
		}
		if key == "" {
			key, err = generateKey()
			if err != nil {
				return fmt.Errorf("generate key: %w", err)
			}
			fmt.Printf("  Generated key: %s\n", key)
			printInfo("save this key — needed to sync on other machines")
		}
		if err := saveSyncKey(key); err != nil {
			return fmt.Errorf("save key: %w", err)
		}
	}

	encrypted, err := encrypt(data, key)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	gistID, _ := syncGistID()

	if gistID != "" {
		printInfo("updating gist %s...", gistID[:8])
		_, err := ghExecWithStdin(
			[]string{"gist", "edit", gistID, "-f", "ghs-config.enc", "-"},
			strings.NewReader(encrypted),
		)
		if err != nil {
			printInfo("gist edit failed, creating new one...")
			gistID = ""
		} else {
			ghUser, _ := ghGetUser()
			printSuccess("config pushed to gist %s (%d accounts)", gistID[:8], len(cfg.Accounts))
			printInfo("gist: https://gist.github.com/%s/%s", ghUser, gistID)
			return nil
		}
	}

	printInfo("creating private gist...")
	result, err := ghExecWithStdin(
		[]string{"gist", "create", "-d", gistDesc, "-f", "ghs-config.enc", "-"},
		strings.NewReader(encrypted),
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

	ghUser, _ := ghGetUser()
	printSuccess("config pushed to gist %s (%d accounts)", gistID[:8], len(cfg.Accounts))
	printInfo("gist: https://gist.github.com/%s/%s", ghUser, gistID)
	return nil
}

func syncPull() error {
	gistID, _ := syncGistID()

	if gistID == "" {
		// Auto-discover gist by listing user's gists
		printInfo("no local gist ID, searching your gists...")
		discovered, _ := discoverSyncGist()
		if discovered != "" {
			gistID = discovered
			saveSyncGistID(gistID)
			printInfo("found gist %s", gistID[:8])
		}
	}

	if gistID == "" {
		// Ask user to paste gist URL/ID
		input, err := readInput("Gist URL or ID (from 'ghs sync push' output): ")
		if err != nil || input == "" {
			return fmt.Errorf("no sync gist. Run 'ghs sync push' on another machine first")
		}
		gistID = extractGistID(input)
		if gistID == "" {
			gistID = strings.TrimSpace(input)
		}
		saveSyncGistID(gistID)
	}

	result, err := ghExec("gist", "view", gistID, "-f", "ghs-config.enc", "--raw")
	if err != nil {
		return fmt.Errorf("cannot read gist: %w", err)
	}

	result = strings.TrimSpace(result)

	key, err := loadSyncKey()
	if err != nil {
		return err
	}

	if key == "" {
		key, err = readInput("Encryption key: ")
		if err != nil {
			return err
		}
		if key == "" {
			return fmt.Errorf("key is required. Use 'ghs sync key' on the original machine")
		}
		if err := saveSyncKey(key); err != nil {
			return fmt.Errorf("save key: %w", err)
		}
	}

	configData, err := decrypt(result, key)
	if err != nil {
		return err
	}

	var cfg Config
	if err := json.Unmarshal(configData, &cfg); err != nil {
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

// discoverSyncGist searches the user's gists for one with our description.
func discoverSyncGist() (string, error) {
	result, err := ghExec("gist", "list", "--json", "id,description", "--limit", "50")
	if err != nil {
		return "", err
	}

	var gists []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(result), &gists); err != nil {
		return "", nil
	}

	for _, g := range gists {
		if strings.Contains(g.Description, "ghs config sync") {
			return g.ID, nil
		}
	}
	return "", nil
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
