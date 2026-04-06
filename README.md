# ghs - Git & GitHub Account Switcher

CLI tool to switch between git identities (user.name, user.email) and GitHub accounts (gh CLI auth) in one command.

## Commands

| Command | Description |
|---------|-------------|
| `ghs add <alias> [-n name] [-e email] [-t token]` | Save account (auto-imports gh token if omitted) |
| `ghs import [--force]` | Import all accounts from gh CLI hosts.yml |
| `ghs remove <alias>` | Delete saved account |
| `ghs use <alias>` | Switch git + gh auth to this account |
| `ghs list` | Show saved accounts |
| `ghs whoami` | Show current git/gh identity + branch |
| `ghs fix <repo> [alias]` | Rewrite commits to correct author + force push |
| `ghs sync push\|pull [alias]` | Sync accounts between machines via private Gist |
| `ghs push [--public]` | Push (auto-create repo if needed) |

### `ghs fix` — Rewrite Commit Authors

Fixes commit authorship and force pushes.

```
ghs fix .                    # current repo, default account
ghs fix . work               # current repo, work account
ghs fix owner/repo           # remote repo, default account
ghs fix owner/repo work      # remote repo, work account
ghs fix https://github.com/owner/repo.git work  # full URL
```

`<repo>` can be `.` (current dir), `owner/repo`, or a full URL.

### `ghs sync` — Cross-Machine Sync

Syncs your account config between machines using a private GitHub Gist.

```
ghs sync push           # upload config to private gist
ghs sync pull           # download config from gist
ghs sync push work      # push using work account's gh auth
```

- Uses current `gh auth` account by default
- Specify `[alias]` to use a different account's gh auth
- Auto-selects if only one account has a token
- Gist ID is saved locally for subsequent syncs

All alias arguments (`use`, `remove`, `fix`, `sync`) support fuzzy matching — prefix and substring:

```
ghs use work       # matches work-laptop
ghs remove pers    # matches personal
```

## Install

**One-line install (Linux/macOS):**
```bash
curl -sL https://raw.githubusercontent.com/ru-yaka/ghs/main/install.sh | bash
```

**From source:**
```bash
go install github.com/ru-yaka/ghs@latest
```

Download binaries from [GitHub Releases](https://github.com/ru-yaka/ghs/releases).

## Build

```sh
go build -o ghs .

# Cross-compile all platforms
make dist
```

## Architecture

```
main.go     CLI entry point, command routing, fix/push logic
config.go   Account CRUD, ~/.ghs/config.json, import from gh CLI
git.go      Git operations (config, commits, reset, push)
gh.go       GitHub CLI operations (auth, token, repo create, hosts.yml parsing)
sync.go     Gist-based config sync between machines
utils.go    Version, prompts, formatting, flag parsing helpers
```

Key design decisions:

- **`git config --global`** — identity switches apply globally, work outside repos
- **`~/.ghs/config.json`** — stores accounts as `{name, email, token, gh_user}` per alias
- **`<git-dir>/ghs-account`** — per-repo default account marker, set after `ghs use`
- **`ghs fix`** — clones to temp dir (if remote), rewrites with `git filter-branch`, force pushes, cleans up
- **`ghs sync`** — stores config in a private Gist, auto-switches gh auth if needed

## Dependencies

- `github.com/cli/go-gh/v2` — gh CLI auth/token/config integration
- `gopkg.in/yaml.v3` — parse gh CLI's hosts.yml

## License

MIT
