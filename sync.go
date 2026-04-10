package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// cmdSync handles: ghs sync export|import
func cmdSync(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs sync export|import")
	}

	switch args[0] {
	case "export":
		return syncExport()
	case "import":
		return syncImport()
	default:
		return fmt.Errorf("usage: ghs sync export|import")
	}
}

func syncExport() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if len(cfg.Accounts) == 0 {
		return fmt.Errorf("no accounts to export. Run 'ghs add' or 'ghs import' first")
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}

	blob, err := encryptBlob(data)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	fmt.Println()
	fmt.Println(blob)
	fmt.Println()
	printSuccess("exported %d account(s)", len(cfg.Accounts))
	printInfo("copy the above text and run 'ghs sync import' on another machine")
	return nil
}

func syncImport() error {
	blob, err := readInput("Paste sync data: ")
	if err != nil {
		return err
	}
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return fmt.Errorf("no data provided")
	}

	data, err := decryptBlob(blob)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	var imported Config
	if err := json.Unmarshal(data, &imported); err != nil {
		return fmt.Errorf("invalid data: %w", err)
	}
	if imported.Accounts == nil {
		imported.Accounts = make(map[string]Account)
	}

	// Load local config
	localCfg, err := loadConfig()
	if err != nil {
		return err
	}
	if localCfg.Accounts == nil {
		localCfg.Accounts = make(map[string]Account)
	}

	// Merge: add new, skip existing
	added, skipped := 0, 0
	for alias, acc := range imported.Accounts {
		if _, exists := localCfg.Accounts[alias]; exists {
			skipped++
			continue
		}
		localCfg.Accounts[alias] = acc
		added++

		// Also login to gh CLI if token available
		if acc.Token != "" && ghIsInstalled() {
			if err := ghLoginWithToken(acc.Token); err != nil {
				printError("gh auth login failed for '%s': %s", alias, err)
			} else {
				printInfo("gh auth: logged in as %s", acc.GhUser)
			}
		}
	}

	if added == 0 {
		printInfo("all %d account(s) already exist locally", skipped)
		return nil
	}

	if err := saveConfig(localCfg); err != nil {
		return err
	}

	printSuccess("imported %d account(s)", added)
	if skipped > 0 {
		printInfo("skipped %d (already exist)", skipped)
	}
	return nil
}
