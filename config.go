package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Account struct {
	Email  string `json:"email"`
	Token  string `json:"token,omitempty"`
	GhUser string `json:"gh_user,omitempty"`
}

type Config struct {
	Accounts map[string]Account `json:"accounts"`
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	return filepath.Join(home, ".ghs"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Accounts: make(map[string]Account)}, nil
		}
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config: %w", err)
	}
	if cfg.Accounts == nil {
		cfg.Accounts = make(map[string]Account)
	}
	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}

	perm := os.FileMode(0600)
	if runtime.GOOS == "windows" {
		perm = 0666
	}

	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("cannot write config: %w", err)
	}
	return nil
}

// addAccount saves an account. If email is empty, reads from git config.
// If token is empty, tries to import from current gh CLI auth.
func addAccount(alias, email, token string) error {
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	// Try git config for email
	if email == "" {
		email, _ = gitConfigGet("user.email")
	}

	// Try to import token from gh CLI
	if token == "" && ghIsInstalled() {
		if t, err := ghGetToken(); err == nil {
			token = t
		}
	}

	if email == "" {
		return fmt.Errorf("email is required (use -e flag or set git user.email)")
	}

	// Optionally get gh username for display
	ghUser := ""
	if ghIsInstalled() {
		ghUser, _ = ghGetUser()
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	cfg.Accounts[alias] = Account{
		Email:  email,
		Token:  token,
		GhUser: ghUser,
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}

	parts := []string{fmt.Sprintf("'%s' added: <%s>", alias, email)}
	if token != "" {
		parts = append(parts, "with token")
	}
	if ghUser != "" {
		parts = append(parts, fmt.Sprintf("(gh: %s)", ghUser))
	}
	fmt.Printf("  ✓ %s\n", strings.Join(parts, " "))

	// Auto-sync to WebDAV
	if webdavIsConfigured() {
		webdavUpload()
	}

	return nil
}

func removeAccount(alias string) error {
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	resolved, err := resolveAlias(alias)
	if err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	delete(cfg.Accounts, resolved)
	if err := saveConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("  ✓ Account '%s' removed\n", resolved)

	// Auto-sync to WebDAV
	if webdavIsConfigured() {
		webdavUpload()
	}

	return nil
}

func clearAllAccounts(sync bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	count := len(cfg.Accounts)
	if count == 0 {
		fmt.Println("No accounts to remove.")
		return nil
	}

	if !confirm(fmt.Sprintf("Remove all %d account(s)?", count)) {
		fmt.Println("Cancelled.")
		return nil
	}

	cfg.Accounts = make(map[string]Account)
	if err := saveConfig(cfg); err != nil {
		return err
	}

	printSuccess("removed %d account(s)", count)

	// Only sync if explicitly requested
	if sync && webdavIsConfigured() {
		webdavUpload()
	}

	return nil
}

func resolveAlias(input string) (string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return "", err
	}

	// Exact match
	if _, ok := cfg.Accounts[input]; ok {
		return input, nil
	}

	input = strings.ToLower(input)
	var prefixMatches []string
	var substrMatches []string

	for alias := range cfg.Accounts {
		lower := strings.ToLower(alias)
		if strings.HasPrefix(lower, input) {
			prefixMatches = append(prefixMatches, alias)
		} else if strings.Contains(lower, input) {
			substrMatches = append(substrMatches, alias)
		}
	}

	if len(prefixMatches) == 1 {
		return prefixMatches[0], nil
	}
	if len(prefixMatches) > 1 {
		return "", fmt.Errorf("'%s' matches multiple accounts: %s", input, strings.Join(prefixMatches, ", "))
	}
	if len(substrMatches) == 1 {
		return substrMatches[0], nil
	}
	if len(substrMatches) > 1 {
		return "", fmt.Errorf("'%s' matches multiple accounts: %s", input, strings.Join(substrMatches, ", "))
	}

	return "", fmt.Errorf("account '%s' not found. Run 'ghs add %s' first", input, input)
}

func getAccount(alias string) (*Account, error) {
	resolved, err := resolveAlias(alias)
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	acc := cfg.Accounts[resolved]
	return &acc, nil
}

func listAccounts() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if len(cfg.Accounts) == 0 {
		fmt.Println("No accounts saved.")
		fmt.Println("  ghs add <alias>          Add from current git/gh config")
		fmt.Println("  ghs import                Import from gh CLI")
		return nil
	}

	fmt.Println("Saved accounts:")
	for alias, acc := range cfg.Accounts {
		tokenStatus := "no token"
		if acc.Token != "" {
			tokenStatus = "has token"
		}
		line := fmt.Sprintf("  %-12s <%s>  [%s]", alias, acc.Email, tokenStatus)
		if acc.GhUser != "" {
			line += fmt.Sprintf("  gh:%s", acc.GhUser)
		}
		fmt.Println(line)
	}
	return nil
}

// importGhAccounts reads gh CLI config and imports all authenticated hosts.
func importGhAccounts(overwrite bool) error {
	if !ghIsInstalled() {
		return fmt.Errorf("gh CLI not installed")
	}

	hosts, err := ghImportHosts()
	if err != nil {
		return fmt.Errorf("cannot read gh config: %w", err)
	}

	if len(hosts) == 0 {
		fmt.Println("No authenticated gh accounts found.")
		return nil
	}

	fmt.Printf("Found %d gh account(s):\n\n", len(hosts))

	cfg, _ := loadConfig()

	imported := 0
	for _, h := range hosts {
		alias := h.User // default alias: gh username
		if alias == "" {
			alias = h.Host
		}

		if _, exists := cfg.Accounts[alias]; exists && !overwrite {
			fmt.Printf("  → %-15s <%s>  [skipped, already exists]\n", alias, alias+"@users.noreply.github.com")
			continue
		}

		// Use gh username and noreply email (don't use git config — it's global, not per-account)
		email := h.User + "@users.noreply.github.com"

		cfg.Accounts[alias] = Account{
			Email:  email,
			Token:  h.Token,
			GhUser: h.User,
		}
		fmt.Printf("  ✓ %-15s <%s>  token:****%s\n", alias, email, truncateToken(h.Token))
		imported++
	}

	if imported > 0 {
		if err := saveConfig(cfg); err != nil {
			return err
		}
	}
	fmt.Printf("\nImported %d account(s).\n", imported)

	// Auto-switch: if active gh user differs from current git identity, offer to switch
	if ghIsInstalled() {
		if activeGhUser, err := ghGetUser(); err == nil && activeGhUser != "" {
			if _, curEmail, _ := getCurrentUser(); curEmail != activeGhUser+"@users.noreply.github.com" {
				if acc, ok := cfg.Accounts[activeGhUser]; ok {
					fmt.Printf("\nActive gh user is '%s' but git identity differs.\n", activeGhUser)
					if confirm("Switch git to match?") {
						gitConfigSet("user.name", activeGhUser)
						gitConfigSet("user.email", acc.Email)
						printSuccess("git → %s <%s>", activeGhUser, acc.Email)
					}
				}
			}
		}
	}

	// Show hint about setting proper name/email
	fmt.Println("\nTip: Edit accounts with proper email:")
	fmt.Println("  ghs add <alias> -e your@email.com")

	// Auto-sync to WebDAV
	if imported > 0 && webdavIsConfigured() {
		webdavUpload()
	}

	return nil
}

func setRepoAccount(alias string) error {
	out, err := gitExec("rev-parse", "--git-dir")
	if err != nil {
		return fmt.Errorf("not a git repository")
	}
	gitDir := strings.TrimSpace(out)
	path := filepath.Join(gitDir, "ghs-account")
	return os.WriteFile(path, []byte(alias), 0644)
}

func getRepoAccount() (string, error) {
	out, err := gitExec("rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	gitDir := strings.TrimSpace(out)
	path := filepath.Join(gitDir, "ghs-account")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no account set for this repository")
	}
	return strings.TrimSpace(string(data)), nil
}

// getDefaultAccount returns the account matching the current global git user.email.
func getDefaultAccount() (*Account, error) {
	email, err := gitConfigGet("user.email")
	if err != nil {
		return nil, fmt.Errorf("cannot get git user.email: %w", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	for alias, acc := range cfg.Accounts {
		if acc.Email == email {
			acc := acc // capture
			_ = alias
			return &acc, nil
		}
	}

	return nil, fmt.Errorf("no account matches current git email '%s'. Use 'ghs fix <alias>' to specify", email)
}

// getAccountByRepoOwner extracts the owner from a repo reference (URL or owner/repo)
// and finds a matching account by GhUser field.
func getAccountByRepoOwner(repo string) (*Account, error) {
	owner := extractOwner(repo)
	if owner == "" {
		return nil, fmt.Errorf("cannot determine repo owner from '%s'", repo)
	}

	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	for alias, acc := range cfg.Accounts {
		if strings.EqualFold(acc.GhUser, owner) {
			acc := acc
			_ = alias
			return &acc, nil
		}
	}

	return nil, fmt.Errorf("no account with gh user '%s'", owner)
}

// extractOwner extracts the owner from a repo reference.
// "https://github.com/owner/repo.git" → "owner"
// "owner/repo" → "owner"
func extractOwner(repo string) string {
	if strings.Contains(repo, "://") {
		// URL: https://github.com/owner/repo.git
		repo = strings.TrimSuffix(repo, ".git")
		repo = strings.TrimRight(repo, "/")
		parts := strings.Split(repo, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}
	// owner/repo format
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

func truncateToken(token string) string {
	if len(token) > 6 {
		return token[len(token)-6:]
	}
	return token
}

// findAliasByEmail finds the account alias matching the given email.
func findAliasByEmail(email string) string {
	cfg, err := loadConfig()
	if err != nil {
		return ""
	}
	for a, acc := range cfg.Accounts {
		if acc.Email == email {
			return a
		}
	}
	return ""
}
