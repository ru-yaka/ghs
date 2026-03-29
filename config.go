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
	Name  string `json:"name"`
	Email string `json:"email"`
	Token string `json:"token,omitempty"`
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

// addAccount saves an account. If name/email are empty, reads from git config.
// If token is empty, tries to import from current gh CLI auth.
func addAccount(alias, name, email, token string) error {
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	// Try git config for name/email
	if name == "" {
		name, _ = gitConfigGet("user.name")
	}
	if email == "" {
		email, _ = gitConfigGet("user.email")
	}

	// Try to import token from gh CLI
	if token == "" && ghIsInstalled() {
		if t, err := ghGetToken(); err == nil {
			token = t
		}
	}

	if name == "" {
		return fmt.Errorf("name is required (use -n flag or set git user.name)")
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
		Name:   name,
		Email:  email,
		Token:  token,
		GhUser: ghUser,
	}

	if err := saveConfig(cfg); err != nil {
		return err
	}

	parts := []string{fmt.Sprintf("'%s' added: %s <%s>", alias, name, email)}
	if token != "" {
		parts = append(parts, "with token")
	}
	if ghUser != "" {
		parts = append(parts, fmt.Sprintf("(gh: %s)", ghUser))
	}
	fmt.Printf("  ✓ %s\n", strings.Join(parts, " "))
	return nil
}

func removeAccount(alias string) error {
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if _, ok := cfg.Accounts[alias]; !ok {
		return fmt.Errorf("account '%s' not found", alias)
	}

	delete(cfg.Accounts, alias)
	if err := saveConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("  ✓ Account '%s' removed\n", alias)
	return nil
}

func getAccount(alias string) (*Account, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	acc, ok := cfg.Accounts[alias]
	if !ok {
		return nil, fmt.Errorf("account '%s' not found. Run 'ghs add %s' first", alias, alias)
	}
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
		line := fmt.Sprintf("  %-12s %s <%s>  [%s]", alias, acc.Name, acc.Email, tokenStatus)
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
			fmt.Printf("  → %-15s %s <%s>  [skipped, already exists]\n", alias, h.User, alias+"@users.noreply.github.com")
			continue
		}

		// Use gh username and noreply email (don't use git config — it's global, not per-account)
		name := h.User
		email := h.User + "@users.noreply.github.com"

		cfg.Accounts[alias] = Account{
			Name:   name,
			Email:  email,
			Token:  h.Token,
			GhUser: h.User,
		}
		fmt.Printf("  ✓ %-15s %s <%s>  token:****%s\n", alias, name, email, truncateToken(h.Token))
		imported++
	}

	if imported > 0 {
		if err := saveConfig(cfg); err != nil {
			return err
		}
	}
	fmt.Printf("\nImported %d account(s).\n", imported)

	// Show hint about setting proper name/email
	fmt.Println("\nTip: Edit accounts with proper git name/email:")
	fmt.Println("  ghs add <alias> -n \"Your Name\" -e your@email.com")
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

func truncateToken(token string) string {
	if len(token) > 6 {
		return token[len(token)-6:]
	}
	return token
}
