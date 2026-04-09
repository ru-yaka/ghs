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
	case "add":
		err = cmdAdd(args)
	case "remove", "rm":
		err = cmdRemove(args)
	case "clear":
		err = cmdClear(args)
	case "use", "switch":
		err = cmdUse(args)
	case "list", "ls":
		err = listAccounts()
	case "repos":
		err = cmdRepos(args)
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

// cmdAdd handles: ghs add <alias> [-e email] [-t token]
func cmdAdd(args []string) error {
	alias, flagArgs := extractAlias(args)
	if alias == "" {
		fmt.Println("Usage: ghs add <alias> [-e email] [-t token]")
		return fmt.Errorf("alias is required")
	}

	fs := flag.NewFlagSet("add", flag.ExitOnError)
	email := fs.String("e", "", "Author email")
	token := fs.String("t", "", "GitHub token")
	fs.Parse(flagArgs)

	return addAccount(alias, *email, *token)
}

// cmdRemove handles: ghs remove <alias>
func cmdRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs remove <alias>")
	}
	return removeAccount(args[0])
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

	// Switch git user — use resolved alias as name
	if err := gitConfigSet("user.name", resolved); err != nil {
		return fmt.Errorf("failed to set git user.name: %w", err)
	}
	if err := gitConfigSet("user.email", acc.Email); err != nil {
		return fmt.Errorf("failed to set git user.email: %w", err)
	}
	printSuccess("git → %s <%s>", resolved, acc.Email)

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
	if err := setRepoAccount(resolved); err == nil {
		printInfo("'%s' set as default for this repository", resolved)
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
	if alias != "" {
		acc, err = getAccount(alias)
	} else if repo != "." {
		// For remote repos, default to account matching the repo owner
		acc, err = getAccountByRepoOwner(repo)
		if err != nil {
			printInfo("%s", err)
			acc, err = getDefaultAccount()
		}
	} else {
		acc, err = getDefaultAccount()
	}
	if err != nil {
		return err
	}

	// Resolve alias for git user.name (use GhUser or alias)
	if alias == "" {
		alias = acc.GhUser
		if alias == "" {
			alias = findAliasByEmail(acc.Email)
		}
	}

	if repo == "." {
		return fixInPlace(alias, acc)
	}
	return cloneAndFix(repo, alias, acc)
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

	// Find commits with wrong author
	var wrongCommits []Commit
	for _, c := range commits {
		if c.AuthorEmail != acc.Email {
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
	_, err = gitExec("filter-branch", "-f", "--env-filter", filterScript, "--", "--all")
	if err != nil {
		return fmt.Errorf("git filter-branch failed: %w", err)
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
		out, err := gitExec("push", "--force", "origin", branch)
		if err != nil {
			return fmt.Errorf("force push failed: %s", out)
		}
		printSuccess("pushed to %s", upstream)
	} else {
		fmt.Println("\n  ⚠ No upstream set. Push manually:")
		fmt.Println("    git push -u origin <branch>")
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

	if err := push(*remoteName, branch, setUpstream); err != nil {
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
