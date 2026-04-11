# ghs - Git & GitHub Account Switcher

CLI tool to switch between git identities (user.name, user.email) and GitHub accounts (gh CLI auth) in one command.

## Commands

| Command | Description |
|---------|-------------|
| `ghs add <name> [-e email] [-t token]` | Save account (auto-imports gh token if omitted) |
| `ghs import [--force]` | Import all accounts from gh CLI |
| `ghs remove <name>` | Delete saved account |
| `ghs use <name>` | Switch git + gh auth to this account |
| `ghs list` | Show saved accounts |
| `ghs repos [name]` | List GitHub repos (all or specific account) |
| `ghs delete <repo...> [--yes]` | Delete one or more GitHub repos |
| `ghs whoami` | Show current git/gh identity + branch |
| `ghs fix <repo> [name]` | Rewrite commits to correct author + push |
| `ghs refresh [name]` | Refresh GitHub token |
| `ghs sync export/import` | Sync accounts between machines |
| `ghs push [--public]` | Push (auto-create repo if needed) |
| `ghs update` | Self-upgrade to latest version |

## Name Argument

The `<name>` argument is an account identifier - either the full GitHub username or a short fragment for quick typing.

```
ghs use ru-yaka      # full name
ghs use ru           # fragment matches ru-yaka
ghs use nick         # fragment matches nickkillie
```

When you use a fragment, ghs finds the matching account and uses the **GitHub username** as `git user.name`, not the fragment.

## Examples

### Switch accounts
```bash
ghs use ru           # switch to ru-yaka
ghs whoami           # shows: git: ru-yaka <...>
```

### Fix commit authors
```bash
ghs fix .            # current repo, default account
ghs fix . ru         # current repo, ru-yaka account
ghs fix owner/repo   # remote repo, default account
```

### List repos
```bash
ghs repos            # all accounts
ghs repos ru         # specific account
```

### Delete repos
```bash
ghs delete test-repo              # delete by name (matches across accounts)
ghs delete owner/repo             # delete by full name
ghs delete repo1 repo2 --yes      # delete multiple, skip confirmation
```

### Sync between machines
```bash
ghs sync export      # copy encrypted data
ghs sync import      # paste on another machine
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

## License

MIT
