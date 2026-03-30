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
| `ghs undo [--all] [--last N] [-y]` | Soft-reset wrong-author commits, keep changes staged |
| `ghs fix <alias> [--all] [--last N] [-y]` | Undo + switch in one step |
| `ghs push [--public] [-r remote]` | Push; auto-create GitHub repo if no remote |

All alias arguments (`use`, `remove`, `fix`) support fuzzy matching — prefix and substring:

```
ghs use nick       # matches nickkillie
ghs remove ru      # matches ru-yaka
```

## Install

Download from [GitHub Releases](https://github.com/ru-yaka/ghs/releases).

**Windows:** Extract zip, run `install-ghs.ps1` (or `install.bat`).

**Linux/macOS:** Copy `ghs` to a directory on `$PATH`, e.g. `/usr/local/bin/`.

## Build

```sh
go build -o ghs .
```

Cross-compile:

```sh
GOOS=windows GOARCH=amd64 go build -o ghs.exe .
GOOS=darwin GOARCH=arm64 go build -o ghs .
```

## Architecture

```
main.go     CLI entry point, command routing, undo/fix/push logic
config.go   Account CRUD, ~/.ghs/config.json, import from gh CLI
git.go      Git operations (config, commits, reset, push)
gh.go       GitHub CLI operations (auth, token, repo create, hosts.yml parsing)
utils.go    Version, prompts, formatting, flag parsing helpers
```

Key design decisions:

- **`git config --global`** — identity switches apply globally, work outside repos
- **`~/.ghs/config.json`** — stores accounts as `{name, email, token, gh_user}` per alias
- **`<git-dir>/ghs-account`** — per-repo default account marker, set after `ghs use`
- **Token import** — parses gh CLI's `hosts.yml` (both old flat and new multi-account format); for multi-account, temporarily `gh auth switch`es to read inactive tokens from OS keyring, then restores original active user
- **Undo** — soft-resets to upstream (default), oldest wrong commit's parent, or empty state if all commits are wrong

## Dependencies

- `github.com/cli/go-gh/v2` — gh CLI auth/token/config integration
- `gopkg.in/yaml.v3` — parse gh CLI's hosts.yml

## License

MIT
