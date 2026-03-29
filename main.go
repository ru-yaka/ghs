package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
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
	case "use", "switch":
		err = cmdUse(args)
	case "list", "ls":
		err = listAccounts()
	case "whoami", "status":
		cmdWhoami()
		os.Exit(0)
	case "import":
		err = cmdImport(args)
	case "undo":
		err = cmdUndo(args)
	case "push":
		err = cmdPush(args)
	case "fix":
		err = cmdFix(args)
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

// cmdAdd handles: ghs add <alias> [-n name] [-e email] [-t token]
func cmdAdd(args []string) error {
	alias, flagArgs := extractAlias(args)
	if alias == "" {
		fmt.Println("Usage: ghs add <alias> [-n name] [-e email] [-t token]")
		return fmt.Errorf("alias is required")
	}

	fs := flag.NewFlagSet("add", flag.ExitOnError)
	name := fs.String("n", "", "Author name")
	email := fs.String("e", "", "Author email")
	token := fs.String("t", "", "GitHub token")
	fs.Parse(flagArgs)

	return addAccount(alias, *name, *email, *token)
}

// cmdRemove handles: ghs remove <alias>
func cmdRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ghs remove <alias>")
	}
	return removeAccount(args[0])
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

	acc, err := getAccount(alias)
	if err != nil {
		return err
	}

	// Switch git user
	if err := gitConfigSet("user.name", acc.Name); err != nil {
		return fmt.Errorf("failed to set git user.name: %w", err)
	}
	if err := gitConfigSet("user.email", acc.Email); err != nil {
		return fmt.Errorf("failed to set git user.email: %w", err)
	}
	printSuccess("git → %s <%s>", acc.Name, acc.Email)

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
	if err := setRepoAccount(alias); err == nil {
		printInfo("'%s' set as default for this repository", alias)
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

// cmdUndo handles: ghs undo [--all] [--last N] [-y]
func cmdUndo(args []string) error {
	fs := flag.NewFlagSet("undo", flag.ExitOnError)
	all := fs.Bool("all", false, "All commits")
	lastN := fs.Int("last", 0, "Only last N commits")
	yes := fs.Bool("y", false, "Skip confirmation")
	fs.Parse(args)

	return doUndo("", *all, *lastN, *yes)
}

// cmdFix handles: ghs fix <alias> [--all] [--last N] [-y]
func cmdFix(args []string) error {
	fs := flag.NewFlagSet("fix", flag.ExitOnError)
	all := fs.Bool("all", false, "All commits")
	lastN := fs.Int("last", 0, "Only last N commits")
	yes := fs.Bool("y", false, "Skip confirmation")

	alias, flagArgs := extractAlias(args)
	if alias == "" {
		fmt.Println("Usage: ghs fix <alias> [--all] [--last N] [-y]")
		return fmt.Errorf("alias is required")
	}
	fs.Parse(flagArgs)

	acc, err := getAccount(alias)
	if err != nil {
		return err
	}

	fmt.Printf("Fix: undo wrong commits → switch to '%s' (%s <%s>)\n\n", alias, acc.Name, acc.Email)

	curName, curEmail, _ := getCurrentUser()
	if curName != "" {
		fmt.Printf("  Current: %s <%s>\n", curName, curEmail)
	}
	fmt.Printf("  Target:  %s <%s>\n\n", acc.Name, acc.Email)

	if err := doUndo(acc.Email, *all, *lastN, *yes); err != nil {
		return err
	}

	fmt.Println()
	return cmdUse([]string{alias})
}

// doUndo is the core undo logic.
// correctEmail: non-empty = commits NOT matching are wrong (used by `fix`).
// Empty = commits by current user are wrong (standalone `undo`).
func doUndo(correctEmail string, allCommits bool, lastN int, skipConfirm bool) error {
	var commits []Commit
	var err error
	var base string

	if allCommits {
		commits, err = getCommits("")
		base = "all commits"
	} else if lastN > 0 {
		commits, err = getCommits(fmt.Sprintf("-n%d", lastN))
		base = fmt.Sprintf("last %d commits", lastN)
	} else if hasUpstream() {
		commits, err = getCommits("@{u}..HEAD")
		base = "unpushed commits"
	} else {
		commits, err = getCommits("")
		base = "all commits (no upstream)"
	}

	if err != nil {
		return fmt.Errorf("cannot get commits: %w", err)
	}
	if len(commits) == 0 {
		printInfo("no commits to undo")
		return nil
	}

	// Determine which email is "wrong"
	var wrongEmail string
	if correctEmail == "" {
		_, curEmail, _ := getCurrentUser()
		wrongEmail = curEmail
	}

	var wrongCommits []Commit
	for _, c := range commits {
		if wrongEmail != "" {
			if c.AuthorEmail == wrongEmail {
				wrongCommits = append(wrongCommits, c)
			}
		} else {
			if c.AuthorEmail != correctEmail {
				wrongCommits = append(wrongCommits, c)
			}
		}
	}

	if len(wrongCommits) == 0 {
		printInfo("no wrong commits found in %d %s", len(commits), base)
		return nil
	}

	fmt.Printf("Found %d commit(s) to undo (out of %d %s):\n\n", len(wrongCommits), len(commits), base)
	for i, c := range wrongCommits {
		fmt.Printf("  #%d  %s  %s <%s>\n      %s\n", i+1, shortHash(c.Hash), c.AuthorName, c.AuthorEmail, c.Subject)
	}
	fmt.Println()

	if !skipConfirm {
		if !confirm("Undo these commits? Changes will be kept staged.") {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Determine reset strategy
	upstreamAvail := hasUpstream()
	useEmptyReset := false
	var resetTarget string

	if upstreamAvail && !allCommits && lastN == 0 {
		resetTarget = "@{u}"
	} else {
		oldestHash := wrongCommits[len(wrongCommits)-1].Hash
		parent, err := gitExec("rev-parse", oldestHash+"^")
		if err != nil {
			useEmptyReset = true
		} else {
			totalCommits, _ := getCommitCount()
			if totalCommits == len(wrongCommits) {
				useEmptyReset = true
			} else {
				resetTarget = parent
			}
		}
	}

	if useEmptyReset {
		printInfo("resetting to empty state (all commits)...")
		if err := resetToEmpty(); err != nil {
			return fmt.Errorf("reset failed: %w", err)
		}
	} else {
		printInfo("resetting to %s...", shortRef(resetTarget))
		if err := softReset(resetTarget); err != nil {
			return fmt.Errorf("reset failed: %w", err)
		}
		if err := stageAll(); err != nil {
			return fmt.Errorf("staging failed: %w", err)
		}
	}

	printSuccess("%d commit(s) undone — changes are staged and ready to commit", len(wrongCommits))
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
