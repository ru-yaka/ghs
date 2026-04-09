package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const repoOwner = "ru-yaka"
const repoNameGHS = "ghs"

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func cmdUpdate(args []string) error {
	target := "latest"
	if len(args) > 0 {
		target = args[0]
	}

	// Current binary path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find current binary: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	// Fetch release info
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoNameGHS)
	if target != "latest" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", repoOwner, repoNameGHS, target)
	}

	printInfo("checking for updates...")
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "ghs/"+version)

	// Use gh auth token if available (avoids 403 rate limit)
	if ghIsInstalled() {
		if token, err := ghGetToken(); err == nil && token != "" {
			req.Header.Set("Authorization", "token "+token)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("cannot parse release info: %w", err)
	}

	// Already up to date?
	if rel.TagName == "v"+version || rel.TagName == version {
		printSuccess("already up to date (%s)", version)
		return nil
	}

	printInfo("new version available: %s (current: v%s)", rel.TagName, version)

	// Find matching asset: linux-amd64, windows-amd64, darwin-arm64, etc
	archPattern := runtime.GOOS + "-" + runtime.GOARCH
	var downloadURL string
	for _, a := range rel.Assets {
		if strings.Contains(strings.ToLower(a.Name), archPattern) {
			downloadURL = a.URL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s in %s", archPattern, rel.TagName)
	}

	// Download with retry
	printInfo("downloading %s...", rel.TagName)

	var tmp *os.File
	var tmpPath string
	var size int64

	for attempt := 1; attempt <= 3; attempt++ {
		tmp, err = os.CreateTemp("", "ghs-update-*")
		if err != nil {
			return fmt.Errorf("cannot create temp file: %w", err)
		}
		tmpPath = tmp.Name()

		resp2, err := http.Get(downloadURL)
		if err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			printInfo("download attempt %d failed: %v", attempt, err)
			if attempt < 3 {
				printInfo("retrying...")
			}
			continue
		}

		if resp2.StatusCode != 200 {
			tmp.Close()
			os.Remove(tmpPath)
			resp2.Body.Close()
			printInfo("download attempt %d failed: %s", attempt, resp2.Status)
			if attempt < 3 {
				printInfo("retrying...")
			}
			continue
		}

		size, err = io.Copy(tmp, resp2.Body)
		resp2.Body.Close()
		tmp.Close()

		if err != nil {
			os.Remove(tmpPath)
			printInfo("download attempt %d failed: %v", attempt, err)
			if attempt < 3 {
				printInfo("retrying...")
			}
			continue
		}

		// Success
		break
	}

	if size == 0 {
		os.Remove(tmpPath)
		return fmt.Errorf("download failed after 3 attempts")
	}
	defer os.Remove(tmpPath)

	printInfo("downloaded %s", formatSize(int(size/1024)))

	// Make executable
	if runtime.GOOS != "windows" {
		os.Chmod(tmpPath, 0755)
	}

	// Replace
	if runtime.GOOS == "windows" {
		// Windows assets are zip files - need to extract
		tmp.Close()
		extractedPath := tmpPath + ".exe"
		if err := extractFromZip(tmpPath, "ghs.exe", extractedPath); err != nil {
			return fmt.Errorf("cannot extract from zip: %w", err)
		}
		os.Remove(tmpPath)
		defer os.Remove(extractedPath)

		// Windows can't replace running executable
		// Download to ghs-new.exe next to current binary
		newPath := filepath.Join(filepath.Dir(exe), "ghs-new.exe")
		if err := os.Rename(extractedPath, newPath); err != nil {
			// If rename fails (cross-device), copy instead
			data, err := os.ReadFile(extractedPath)
			if err != nil {
				return fmt.Errorf("cannot read extracted file: %w", err)
			}
			if err := os.WriteFile(newPath, data, 0755); err != nil {
				return fmt.Errorf("cannot write new binary: %w", err)
			}
		}
		printSuccess("downloaded to %s", newPath)
		fmt.Println("  Please close ghs and manually replace:")
		fmt.Printf("    del \"%s\"\n", exe)
		fmt.Printf("    ren \"%s\" ghs.exe\n", newPath)
		return nil
	}

	// Unix: try direct replace, fall back to sudo
	if err := os.Rename(tmpPath, exe); err != nil {
		printInfo("cannot replace %s — trying with elevated permissions...", exe)
		cmd := exec.Command("sudo", "cp", tmpPath, exe)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cannot replace binary: %s (%w)", string(out), err)
		}
	}

	printSuccess("updated to %s", rel.TagName)
	return nil
}

// extractFromZip extracts a specific file from a zip archive.
func extractFromZip(zipPath, filename, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) == filename {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("file %s not found in zip", filename)
}
