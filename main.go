package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	cleanupLegacyFiles()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "remove", "rm":
		err = cmdRemove(args)
	case "add":
		err = cmdAdd(args)
	case "clear":
		err = cmdClear(args)
	case "use", "switch":
		err = cmdUse(args)
	case "git":
		err = cmdGit(args)
	case "list", "ls":
		err = listAccounts()
	case "delete":
		err = cmdDelete(args)
	case "whoami", "status":
		cmdWhoami()
		os.Exit(0)
	case "import":
		err = cmdImport(args)
	case "push":
		err = cmdPush(args)
	case "fix":
		err = cmdFix(args)
	case "sync":
		err = cmdSync(args)
	case "apply":
		err = cmdApply(args)
	case "update", "upgrade":
		err = cmdUpdate(args)
	case "help", "--help", "-h":
		printUsage()
	case "version", "--version", "-v":
		fmt.Printf("ghs v%s\n", version)
	default:
		printError("unknown command: %s", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		printError("%s", err)
		os.Exit(1)
	}
}

// cmdRemove handles: ghs remove <alias>
func cmdRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs remove <alias>")
	}
	return removeAccount(args[0])
}

// cmdAdd handles: ghs add <name> [-e email] [-t token]
func cmdAdd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs add <name> [-e email] [-t token]")
	}

	name := args[0]
	email := ""
	token := ""

	// Parse flags
	for i := 1; i < len(args); i++ {
		if args[i] == "-e" || args[i] == "--email" {
			if i+1 < len(args) {
				email = args[i+1]
				i++
			}
		} else if args[i] == "-t" || args[i] == "--token" {
			if i+1 < len(args) {
				token = args[i+1]
				i++
			}
		}
	}

	// Load existing config
	cfg, _ := loadConfig()
	if cfg.Accounts == nil {
		cfg.Accounts = make(map[string]Account)
	}

	// Check if account exists
	if _, exists := cfg.Accounts[name]; exists {
		// Update existing account
		acc := cfg.Accounts[name]
		if email != "" {
			acc.Email = email
		}
		if token != "" {
			acc.Token = token
		}
		cfg.Accounts[name] = acc
		printInfo("updated account '%s'", name)
	} else {
		// Create new account
		acc := Account{
			Email:  email,
			Token:  token,
			GhUser: name,
		}
		cfg.Accounts[name] = acc
		printSuccess("added account '%s'", name)
	}

	if email == "" {
		printInfo("set email with: ghs add %s -e your@email.com", name)
	}

	return saveConfig(cfg)
}

// cmdClear handles: ghs clear
func cmdClear(args []string) error {
	return clearAllAccounts()
}

// cmdImport handles: ghs import [--force]
func cmdImport(args []string) error {
	force := false
	for _, a := range args {
		if a == "--force" || a == "-f" {
			force = true
		}
	}
	return importGhAccounts(force)
}

// cmdApply handles: ghs apply
// Syncs current gh user info (name, email) to git config.
// Fetches the latest info from GitHub API.
func cmdApply(args []string) error {
	if !ghIsInstalled() {
		return fmt.Errorf("gh CLI not installed")
	}

	ghUser, err := ghGetUser()
	if err != nil {
		return fmt.Errorf("cannot get current gh user: %w", err)
	}

	// git user.name = GitHub login (username)
	gitName := ghUser

	// Fetch email: try user/emails API, then build noreply from user id
	email, err := ghGetUserNoreplyEmail()
	if err != nil {
		return fmt.Errorf("cannot determine email: %w", err)
	}

	// Load saved config for update
	cfg, _ := loadConfig()

	// Check current git config
	curName, curEmail, _ := getCurrentUser()
	if curName == gitName && curEmail == email {
		printSuccess("git config already matches: %s <%s>", gitName, email)
		return nil
	}

	// Show changes
	if curName != gitName {
		printInfo("user.name: %s → %s", curName, gitName)
	}
	if curEmail != email {
		printInfo("user.email: %s → %s", curEmail, email)
	}

	// Set git config
	if err := gitConfigSet("user.name", gitName); err != nil {
		return fmt.Errorf("failed to set git user.name: %w", err)
	}
	if err := gitConfigSet("user.email", email); err != nil {
		return fmt.Errorf("failed to set git user.email: %w", err)
	}

	printSuccess("git → %s <%s>", gitName, email)

	// Update saved account
	updated := false
	for alias, acc := range cfg.Accounts {
		if acc.GhUser == ghUser {
			acc.Email = email
			cfg.Accounts[alias] = acc
			updated = true
			break
		}
	}
	if updated {
		saveConfig(cfg)
		printInfo("updated saved account")
	}

	return nil
}

// cmdGit handles: ghs git <alias>
// Switches only git identity (user.name, user.email), not gh auth.
func cmdGit(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs git <alias>")
	}
	alias := args[0]

	resolved, err := resolveAlias(alias)
	if err != nil {
		return err
	}

	acc, err := getAccount(alias)
	if err != nil {
		return err
	}

	// Use GitHub username as git user.name (not the alias)
	gitUserName := acc.GhUser
	if gitUserName == "" {
		gitUserName = resolved
	}

	// Switch git user only
	if err := gitConfigSet("user.name", gitUserName); err != nil {
		return fmt.Errorf("failed to set git user.name: %w", err)
	}
	if err := gitConfigSet("user.email", acc.Email); err != nil {
		return fmt.Errorf("failed to set git user.email: %w", err)
	}
	printSuccess("git → %s <%s>", gitUserName, acc.Email)

	// Remember for this repo
	if err := setRepoAccount(gitUserName); err == nil {
		printInfo("'%s' set as default for this repository", gitUserName)
	}

	return nil
}

// cmdUse handles: ghs use <alias>
func cmdUse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs use <alias>")
	}
	alias := args[0]

	resolved, err := resolveAlias(alias)
	if err != nil {
		return err
	}

	acc, err := getAccount(alias)
	if err != nil {
		return err
	}

	// Use GitHub username as git user.name (not the alias)
	gitUserName := acc.GhUser
	if gitUserName == "" {
		gitUserName = resolved
	}

	// Switch git user
	if err := gitConfigSet("user.name", gitUserName); err != nil {
		return fmt.Errorf("failed to set git user.name: %w", err)
	}
	if err := gitConfigSet("user.email", acc.Email); err != nil {
		return fmt.Errorf("failed to set git user.email: %w", err)
	}
	printSuccess("git → %s <%s>", gitUserName, acc.Email)

	// Switch gh auth if token available
	if acc.Token != "" {
		if ghIsInstalled() {
			if err := ghLoginWithToken(acc.Token); err != nil {
				printError("gh auth switch failed: %s", err)
				printInfo("git user switched but gh auth unchanged")
				return nil
			}
			ghUser, _ := ghGetUser()
			if ghUser != "" {
				printSuccess("gh  → %s", ghUser)
			}
		} else {
			printInfo("gh CLI not found, skipping gh auth switch")
		}
	} else {
		printInfo("no token saved for '%s'", alias)
	}

	// Remember for this repo
	if err := setRepoAccount(gitUserName); err == nil {
		printInfo("'%s' set as default for this repository", gitUserName)
	}

	return nil
}

// cmdWhoami shows current git and gh identity.
func cmdWhoami() {
	name, email, err := getCurrentUser()
	if err != nil {
		printError("cannot get git identity: %s", err)
	} else {
		fmt.Printf("git:  %s <%s>\n", name, email)
	}

	repoAcc, err := getRepoAccount()
	if err == nil {
		fmt.Printf("repo: %s\n", repoAcc)
	}

	if ghIsInstalled() {
		ghUser, err := ghGetUser()
		if err != nil {
			fmt.Println("gh:   not authenticated")
		} else {
			fmt.Printf("gh:   %s\n", ghUser)
		}
	} else {
		fmt.Println("gh:   not installed")
	}

	branch, err := getCurrentBranch()
	if err == nil {
		fmt.Printf("branch: %s", branch)
		if hasUpstream() {
			upstream, _ := gitExec("rev-parse", "--abbrev-ref", "@{u}")
			fmt.Printf(" → %s", upstream)
		} else {
			fmt.Print(" (no upstream)")
		}
		fmt.Println()
	}
}

// repoPattern matches "owner/repo" format.
var repoPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9._-]+$`)

// isRepoRef returns true if the argument looks like a repo: URL, owner/repo, or "." for cwd.
func isRepoRef(s string) bool {
	return s == "." || strings.Contains(s, "://") || repoPattern.MatchString(s)
}

// cmdFix handles:
//
//	ghs fix <repo> [alias]        fix repo ("." = current dir), alias optional
func cmdFix(args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("usage: ghs fix <repo> [alias]\n  repo: URL, owner/repo, or \".\" for current dir")
	}

	repo := args[0]
	var alias string
	if len(args) == 2 {
		alias = args[1]
	}

	// "." means current directory
	if repo == "." {
		if !isGitRepo() {
			return fmt.Errorf("not in a git repo")
		}
	}

	// Resolve account
	var acc *Account
	var err error
	var resolvedAlias string // The resolved alias (e.g., "ru-yaka" from "ru")
	if alias != "" {
		resolvedAlias, err = resolveAlias(alias)
		if err != nil {
			return err
		}
		acc, err = getAccount(resolvedAlias)
	} else if repo != "." {
		// For remote repos, default to account matching the repo owner
		acc, err = getAccountByRepoOwner(repo)
		if err != nil {
			printInfo("%s", err)
			acc, err = getDefaultAccount()
		}
		if acc != nil {
			resolvedAlias = acc.GhUser
			if resolvedAlias == "" {
				resolvedAlias = findAliasByEmail(acc.Email)
			}
		}
	} else {
		acc, err = getDefaultAccount()
		if acc != nil {
			resolvedAlias = acc.GhUser
			if resolvedAlias == "" {
				resolvedAlias = findAliasByEmail(acc.Email)
			}
		}
	}
	if err != nil {
		return err
	}

	// Use GitHub username as git user.name (not the alias fragment)
	gitUserName := acc.GhUser
	if gitUserName == "" {
		gitUserName = resolvedAlias
	}

	if repo == "." {
		return fixInPlace(gitUserName, acc)
	}
	return cloneAndFix(repo, gitUserName, acc)
}

// cloneAndFix clones a remote repo to a temp directory, fixes it, pushes, and cleans up.
func cloneAndFix(repo, alias string, acc *Account) error {
	// Normalize to owner/repo format for gh repo clone
	cloneRef := repo
	if strings.Contains(repo, "://") {
		// https://github.com/owner/repo.git → owner/repo
		parts := strings.Split(strings.TrimRight(strings.TrimSuffix(repo, ".git"), "/"), "/")
		if len(parts) >= 2 {
			cloneRef = parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ghs-fix-*")
	if err != nil {
		return fmt.Errorf("cannot create temp dir: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(tmpDir)
		}
	}()

	// Switch gh auth to the account that has push access, then clone
	if acc.Token != "" && ghIsInstalled() {
		if err := ghLoginWithToken(acc.Token); err != nil {
			printError("gh auth switch failed: %s", err)
		}
	}

	// Show repo size before cloning
	if ghIsInstalled() {
		if size, err := ghRepoSize(cloneRef); err == nil && size > 0 {
			printInfo("repo size: %s", formatSize(size))
		}
	}

	// Clone with progress output streamed to terminal
	printInfo("cloning %s...", repo)
	if ghIsInstalled() {
		if err := ghCloneWithProgress(cloneRef, tmpDir); err != nil {
			return fmt.Errorf("gh repo clone failed: %w", err)
		}
	} else {
		cloneURL := repo
		if !strings.Contains(repo, "://") {
			cloneURL = "https://github.com/" + repo + ".git"
		}
		if err := gitCloneWithProgress(cloneURL, tmpDir); err != nil {
			return fmt.Errorf("git clone failed: %w", err)
		}
	}

	// Save and switch directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		return fmt.Errorf("cannot chdir: %w", err)
	}
	defer os.Chdir(origDir)

	// Fix
	if err := fixInPlace(alias, acc); err != nil {
		// Don't cleanup on fix failure so user can inspect
		cleanup = false
		printInfo("repo left at %s for inspection", tmpDir)
		return err
	}

	return nil
}

// fixInPlace rewrites all commit authors in the current repo and force pushes.
func fixInPlace(alias string, acc *Account) error {
	// Get all commits
	commits, err := getCommits("")
	if err != nil {
		return fmt.Errorf("cannot get commits: %w", err)
	}
	if len(commits) == 0 {
		printInfo("no commits to fix")
		return nil
	}

	// Find commits with wrong author (check both name and email)
	var wrongCommits []Commit
	for _, c := range commits {
		if c.AuthorEmail != acc.Email || c.AuthorName != alias {
			wrongCommits = append(wrongCommits, c)
		}
	}

	if len(wrongCommits) == 0 {
		printInfo("all commits already have correct author")
		// Still switch identity
		if err := gitConfigSet("user.name", alias); err != nil {
			return fmt.Errorf("failed to set git user.name: %w", err)
		}
		if err := gitConfigSet("user.email", acc.Email); err != nil {
			return fmt.Errorf("failed to set git user.email: %w", err)
		}
		printSuccess("git → %s <%s>", alias, acc.Email)
		return nil
	}

	fmt.Printf("Fix: rewrite %d commit(s) to '%s <%s>'\n\n", len(wrongCommits), alias, acc.Email)
	for i, c := range wrongCommits {
		fmt.Printf("  #%d  %s  %s <%s>\n      %s\n", i+1, shortHash(c.Hash), c.AuthorName, c.AuthorEmail, c.Subject)
	}
	fmt.Println()

	if !confirm("Rewrite author and force push?") {
		fmt.Println("Cancelled.")
		return nil
	}

	// Stash any uncommitted changes
	stashed := false
	if out, _ := gitExec("status", "--porcelain"); out != "" {
		printInfo("stashing uncommitted changes...")
		if _, err := gitExec("stash", "--include-untracked"); err != nil {
			return fmt.Errorf("cannot stash changes: %w", err)
		}
		stashed = true
	}

	// Use git filter-branch to rewrite author
	filterScript := fmt.Sprintf(
		`if [ "$GIT_AUTHOR_EMAIL" != "%s" ]; then `+
			`export GIT_AUTHOR_NAME="%s"; `+
			`export GIT_AUTHOR_EMAIL="%s"; `+
			`fi; `+
			`if [ "$GIT_COMMITTER_EMAIL" != "%s" ]; then `+
			`export GIT_COMMITTER_NAME="%s"; `+
			`export GIT_COMMITTER_EMAIL="%s"; `+
			`fi`,
		acc.Email, alias, acc.Email,
		acc.Email, alias, acc.Email,
	)

	printInfo("rewriting commits...")
	_, err = gitExec("-c", "advice.detachedHead=false", "filter-branch", "-f", "--env-filter", filterScript, "--", "--all")
	if err != nil {
		// Restore stashed changes on failure
		if stashed {
			gitExec("stash", "pop")
		}
		return fmt.Errorf("git filter-branch failed: %w", err)
	}

	// Restore stashed changes
	if stashed {
		printInfo("restoring stashed changes...")
		if _, err := gitExec("stash", "pop"); err != nil {
			printError("failed to restore stash: %s", err)
			printInfo("run 'git stash pop' manually")
		}
	}

	printSuccess("%d commit(s) rewritten", len(wrongCommits))

	// Switch git identity
	if err := gitConfigSet("user.name", alias); err != nil {
		return fmt.Errorf("failed to set git user.name: %w", err)
	}
	if err := gitConfigSet("user.email", acc.Email); err != nil {
		return fmt.Errorf("failed to set git user.email: %w", err)
	}
	printSuccess("git → %s <%s>", alias, acc.Email)

	// Switch gh auth if token available
	if acc.Token != "" && ghIsInstalled() {
		if err := ghLoginWithToken(acc.Token); err != nil {
			printError("gh auth switch failed: %s", err)
		} else {
			ghUser, _ := ghGetUser()
			if ghUser != "" {
				printSuccess("gh  → %s", ghUser)
			}
		}
	}

	// Force push
	if hasUpstream() {
		upstream, _ := gitExec("rev-parse", "--abbrev-ref", "@{u}")
		branch, _ := getCurrentBranch()
		printInfo("force pushing to %s...", upstream)
		_, err = gitExec("push", "--force", "origin", branch)
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "not found") || strings.Contains(errStr, "Repository not found") {
				printError("no access to this repo with current account")
				printInfo("repo may be private or owned by another account")
				printInfo("switch to the repo owner and try: ghs fix .")
			}
			return fmt.Errorf("force push failed: %s", err)
		}
		printSuccess("pushed to %s", upstream)
	} else {
		// No upstream - try to create repo and push
		branch, _ := getCurrentBranch()
		if ghIsInstalled() && acc.Token != "" {
			printInfo("no upstream - creating repo and pushing...")
			// Use cmdPush logic
			if !hasRemote("origin") {
				repoName, err := getRepoName()
				if err != nil {
					repoName = "my-project"
				}
				url, err := ghCreateRepo(repoName, "private", "origin")
				if err != nil {
					return fmt.Errorf("failed to create repo: %w", err)
				}
				printSuccess("repo created: %s", url)
			}
			_, err = gitExec("push", "-u", "origin", branch)
			if err != nil {
				return fmt.Errorf("push failed: %s", err)
			}
			printSuccess("pushed to origin/%s", branch)
		} else {
			fmt.Println("\n  ⚠ No upstream set and no gh CLI/token. Push manually:")
			fmt.Println("    git push -u origin <branch>")
		}
	}

	return nil
}

// cmdPush handles: ghs push [--public] [-r remote]
func cmdPush(args []string) error {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	isPublic := fs.Bool("public", false, "Create public repo")
	remoteName := fs.String("r", "origin", "Remote name")
	fs.Parse(args)

	remoteExists := hasRemote(*remoteName)

	if !remoteExists {
		if !ghIsInstalled() {
			return fmt.Errorf("no remote '%s' and gh CLI not installed — add remote manually:\n  git remote add %s <url>", *remoteName, *remoteName)
		}

		repoName, err := getRepoName()
		if err != nil {
			repoName = "my-project"
		}

		visibility := "private"
		if *isPublic {
			visibility = "public"
		}

		printInfo("no remote '%s' — creating GitHub repo '%s' (%s)...", *remoteName, repoName, visibility)

		url, err := ghCreateRepo(repoName, visibility, *remoteName)
		if err != nil {
			return fmt.Errorf("failed to create repo: %w", err)
		}
		printSuccess("repo created: %s", url)
	}

	branch, err := getCurrentBranch()
	if err != nil {
		return fmt.Errorf("cannot get current branch: %w", err)
	}

	count, err := getCommitCount()
	if err != nil || count == 0 {
		printInfo("nothing to push (no commits)")
		return nil
	}

	setUpstream := !hasUpstream()
	pushLabel := fmt.Sprintf("%s/%s", *remoteName, branch)
	if setUpstream {
		printInfo("pushing to %s (setting upstream)...", pushLabel)
	} else {
		printInfo("pushing to %s...", pushLabel)
	}

	if err := push(*remoteName, branch, setUpstream, true); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "Repository not found") {
			printError("no access to remote repo with current account")
			printInfo("repo may be private or owned by another account")
			printInfo("switch to the repo owner and try again")
			return err
		}
		return err
	}

	printSuccess("pushed to %s", pushLabel)

	if ghIsInstalled() {
		if repoURL, err := ghGetRepoURL(); err == nil {
			printInfo("repo: %s", repoURL)
		}
	}

	return nil
}
