package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// WebDAVConfig holds WebDAV connection settings.
type WebDAVConfig struct {
	URL      string `json:"url"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// webdavConfigPath returns the path to webdav.json.
func webdavConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ghs", "webdav.json"), nil
}

// loadWebDAVConfig loads WebDAV config from file.
func loadWebDAVConfig() (*WebDAVConfig, error) {
	path, err := webdavConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No config = WebDAV not configured
		}
		return nil, err
	}

	var cfg WebDAVConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid webdav.json: %w", err)
	}
	return &cfg, nil
}

// saveWebDAVConfig saves WebDAV config to file.
func saveWebDAVConfig(cfg *WebDAVConfig) error {
	path, err := webdavConfigPath()
	if err != nil {
		return err
	}

	// Ensure .ghs directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// webdavIsConfigured returns true if WebDAV is set up.
func webdavIsConfigured() bool {
	cfg, err := loadWebDAVConfig()
	return err == nil && cfg != nil && cfg.URL != ""
}

// webdavUpload uploads encrypted config to WebDAV.
func webdavUpload() error {
	cfg, err := loadWebDAVConfig()
	if err != nil {
		return err
	}
	if cfg == nil || cfg.URL == "" {
		return nil // Not configured, skip silently
	}

	// Load and encrypt config
	ghsCfg, err := loadConfig()
	if err != nil {
		return err
	}

	data, err := json.Marshal(ghsCfg)
	if err != nil {
		return err
	}

	blob, err := encryptBlob(data)
	if err != nil {
		return err
	}

	// Upload to WebDAV
	url := strings.TrimSuffix(cfg.URL, "/") + "/ghs-config.enc"
	req, err := http.NewRequest("PUT", url, strings.NewReader(blob))
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.User, cfg.Password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("WebDAV upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("WebDAV upload failed: %s", resp.Status)
	}

	return nil
}

// webdavDownload downloads and decrypts config from WebDAV.
func webdavDownload() error {
	cfg, err := loadWebDAVConfig()
	if err != nil {
		return err
	}
	if cfg == nil || cfg.URL == "" {
		return nil // Not configured, skip silently
	}

	url := strings.TrimSuffix(cfg.URL, "/") + "/ghs-config.enc"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.User, cfg.Password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("WebDAV download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil // No remote config yet
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("WebDAV download failed: %s", resp.Status)
	}

	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	data, err := decryptBlob(string(blob))
	if err != nil {
		return fmt.Errorf("decrypt failed: %w", err)
	}

	var remoteCfg Config
	if err := json.Unmarshal(data, &remoteCfg); err != nil {
		return fmt.Errorf("invalid remote config: %w", err)
	}

	// Merge with local config
	localCfg, err := loadConfig()
	if err != nil {
		return err
	}
	if localCfg.Accounts == nil {
		localCfg.Accounts = make(map[string]Account)
	}

	added, updated := 0, 0
	for alias, acc := range remoteCfg.Accounts {
		if local, exists := localCfg.Accounts[alias]; !exists {
			localCfg.Accounts[alias] = acc
			added++
		} else if local.Token == "" && acc.Token != "" {
			// Update token if local is missing
			localCfg.Accounts[alias] = acc
			updated++
		}
	}

	if added > 0 || updated > 0 {
		if err := saveConfig(localCfg); err != nil {
			return err
		}
		printSuccess("synced from WebDAV: %d added, %d updated", added, updated)
	}

	return nil
}

// cmdWebDAV handles: ghs webdav setup|sync|status
func cmdWebDAV(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs webdav setup|sync|status")
	}

	switch args[0] {
	case "setup":
		return webdavSetup()
	case "sync":
		return webdavSync()
	case "status":
		return webdavStatus()
	default:
		return fmt.Errorf("usage: ghs webdav setup|sync|status")
	}
}

func webdavSetup() error {
	const defaultURL = "https://dav.jianguoyun.com/dav/"

	fmt.Println("Configure WebDAV for automatic sync")
	fmt.Println()

	url, err := readInput(fmt.Sprintf("WebDAV URL [default: %s]: ", defaultURL))
	if err != nil {
		return err
	}
	url = strings.TrimSpace(url)
	if url == "" {
		url = defaultURL
	}

	user, err := readInput("Username (or email for Jianguoyun): ")
	if err != nil {
		return err
	}

	password, err := readInput("Password (app password for Jianguoyun): ")
	if err != nil {
		return err
	}

	cfg := &WebDAVConfig{
		URL:      url,
		User:     strings.TrimSpace(user),
		Password: strings.TrimSpace(password),
	}

	// Test connection
	printInfo("testing connection...")
	req, err := http.NewRequest("OPTIONS", cfg.URL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.User, cfg.Password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("authentication failed: %s", resp.Status)
	}

	if err := saveWebDAVConfig(cfg); err != nil {
		return err
	}

	printSuccess("WebDAV configured successfully")
	fmt.Printf("  File: %s/ghs-config.enc\n", cfg.URL)
	printInfo("run 'ghs webdav sync' to sync accounts with remote")
	return nil
}

func webdavSync() error {
	if !webdavIsConfigured() {
		return fmt.Errorf("WebDAV not configured. Run 'ghs webdav setup' first")
	}

	// Load local config
	localCfg, err := loadConfig()
	if err != nil {
		return err
	}
	if localCfg.Accounts == nil {
		localCfg.Accounts = make(map[string]Account)
	}

	// Download remote config
	cfg, err := loadWebDAVConfig()
	if err != nil {
		return err
	}

	url := strings.TrimSuffix(cfg.URL, "/") + "/ghs-config.enc"
	req, _ := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(cfg.User, cfg.Password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("WebDAV download failed: %w", err)
	}
	defer resp.Body.Close()

	var remoteCfg Config
	if resp.StatusCode == 200 {
		blob, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		data, err := decryptBlob(string(blob))
		if err != nil {
			return fmt.Errorf("decrypt failed: %w", err)
		}
		if err := json.Unmarshal(data, &remoteCfg); err != nil {
			return fmt.Errorf("invalid remote config: %w", err)
		}
	}
	if remoteCfg.Accounts == nil {
		remoteCfg.Accounts = make(map[string]Account)
	}

	// Merge: pull new accounts from remote, push local-only accounts
	pulled, pushed := 0, 0
	changed := false

	// Pull: remote accounts not in local
	for alias, acc := range remoteCfg.Accounts {
		if _, exists := localCfg.Accounts[alias]; !exists {
			localCfg.Accounts[alias] = acc
			pulled++
			changed = true
		}
	}

	// Push: local accounts not in remote
	for alias, acc := range localCfg.Accounts {
		if _, exists := remoteCfg.Accounts[alias]; !exists {
			remoteCfg.Accounts[alias] = acc
			pushed++
		}
	}

	// Save local if changed
	if changed {
		if err := saveConfig(localCfg); err != nil {
			return err
		}
	}

	// Upload if there are local-only accounts
	if pushed > 0 {
		data, err := json.Marshal(localCfg)
		if err != nil {
			return err
		}
		blob, err := encryptBlob(data)
		if err != nil {
			return err
		}

		req2, _ := http.NewRequest("PUT", url, strings.NewReader(blob))
		req2.SetBasicAuth(cfg.User, cfg.Password)

		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			return fmt.Errorf("WebDAV upload failed: %w", err)
		}
		resp2.Body.Close()

		if resp2.StatusCode >= 400 {
			return fmt.Errorf("WebDAV upload failed: %s", resp2.Status)
		}
	}

	if pulled == 0 && pushed == 0 {
		printSuccess("already in sync")
	} else {
		if pulled > 0 {
			printSuccess("pulled %d account(s) from remote", pulled)
		}
		if pushed > 0 {
			printSuccess("pushed %d account(s) to remote", pushed)
		}
	}

	return nil
}

func webdavStatus() error {
	cfg, err := loadWebDAVConfig()
	if err != nil {
		return err
	}

	if cfg == nil || cfg.URL == "" {
		fmt.Println("WebDAV: not configured")
		fmt.Println("  Run 'ghs webdav setup' to enable automatic sync")
		return nil
	}

	fmt.Printf("WebDAV: configured\n")
	fmt.Printf("  URL: %s\n", cfg.URL)
	fmt.Printf("  User: %s\n", cfg.User)
	return nil
}
