# ghs - Git & GitHub Account Switcher

CLI tool to switch between git identities (user.name, user.email) and GitHub accounts (gh CLI auth) in one command.

## Commands

| Command | Description |
|---------|-------------|
| `ghs import [--force]` | Import all accounts from gh CLI |
| `ghs add <name> [-e email] [-t token]` | Add or update account |
| `ghs remove <name>` | Delete saved account |
| `ghs use <name>` | Switch git + gh auth to this account |
| `ghs use git:<name>` | Switch git identity only (not gh auth) |
| `ghs list` | Show saved accounts |
| `ghs delete users <user...> [--yes]` | Remove saved accounts |
| `ghs whoami` | Show current git/gh identity + branch |
| `ghs fix <repo> [name]` | Rewrite commits to correct author + push |
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

### Delete saved users
```bash
ghs delete users nick         # remove account (fragment match)
ghs delete users ru nick --yes  # remove multiple, skip confirmation
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
