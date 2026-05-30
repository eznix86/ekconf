<p align="center">
  <pre align="center">
              oooo                                         .o88o.
             `888                                         888 `"
  .ooooo.   888  oooo   .ooooo.   .ooooo.  ooo. .oo.   o888oo
d88' `88b  888 .8P'   d88' `"Y8 d88' `88b `888P"Y88b   888
888ooo888  888888.    888       888   888  888   888   888
888    .o  888 `88b.  888   .o8 888   888  888   888   888
 `Y8bod8P' o888o o888o `Y8bod8P' `Y8bod8P' o888o o888o o888o
  </pre>
  <p align="center">Encrypted kubeconfig manager.</p>
</p>

<p align="center">
  <a href="https://github.com/eznix86/ekconf/releases/latest"><img src="https://img.shields.io/github/v/release/eznix86/ekconf" alt="Latest release"></a>
  <a href="https://github.com/eznix86/ekconf/actions/workflows/lint.yml"><img src="https://img.shields.io/github/actions/workflow/status/eznix86/ekconf/lint.yml?label=lint" alt="Lint"></a>
  <a href="https://github.com/eznix86/ekconf/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/eznix86/ekconf/release.yml?label=release" alt="Release"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/eznix86/ekconf" alt="Go version">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/eznix86/ekconf" alt="License"></a>
</p>

Encrypted kubeconfig manager. Inspired by
[`particledecay/kconf`](https://github.com/particledecay/kconf) which stores
kubeconfigs in plaintext. `ekconf` keeps them encrypted at rest using
AES-256-GCM and Argon2id, with optional macOS Keychain and Linux Keyring
integration.

## Installation

```sh
curl -fsSL https://raw.githubusercontent.com/eznix86/ekconf/main/install.sh | bash
```

Or install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/eznix86/ekconf/main/install.sh | bash -s -- 1.2.3
```

Or via Go:

```sh
go install github.com/eznix86/ekconf@latest
```

Or build from source:

```sh
git clone https://github.com/eznix86/ekconf
cd ekconf
go install .
```

## Commands

| Command | Description |
|---|---|
| `ekconf add <path>` | Encrypt and merge a kubeconfig |
| `ekconf rm <name>` | Remove a context |
| `ekconf ls` | List all contexts |
| `ekconf view <name>` | View a context's kubeconfig (redacted by default) |
| `ekconf view <name> --plain` | Include sensitive auth data |
| `ekconf use <name>` | Set the active context |
| `ekconf ns <namespace>` | Set default namespace on the active context |
| `ekconf exec [<name>] -- <cmd>` | Run a command with decrypted KUBECONFIG |
| `ekconf rotate` | Re-encrypt with a new password |
| `ekconf eject [--force]` | Decrypt and write to `~/.kube/config` |
| `ekconf config list` | View current configuration |
| `ekconf config <key=value>` | Set a configuration option |
| `ekconf update` | Self-update from GitHub Releases |
| `ekconf update --check` | Check for updates without installing |

## Usage

```sh
# Add a kubeconfig
ekconf add ~/.kube/config
ekconf add /path/to/some/other.conf -n my-cluster

# Remove a context
ekconf rm my-cluster

# List all contexts
ekconf ls

# Set the active context
ekconf use my-cluster

# Set default namespace
ekconf ns my-namespace

# Run a command with decrypted config injected via KUBECONFIG
ekconf exec -- kubectl get pods
ekconf exec staging -- kubectl get pods
```

### Password resolution

Checked in order:

1. `--password=<value>` flag
2. `--password-stdin` flag
3. `EKCONF_PASSWORD` environment variable
4. System keychain (if `keychain=true`)
5. Interactive prompt

Enable keychain storage:

```sh
ekconf config keychain=true
```

### Self-update

```sh
ekconf update
ekconf update --check
```

### Shell completion

One-shot zsh session:

```sh
source <(ekconf completion zsh)
```

Persistent:

```sh
ekconf completion zsh > ~/.zsh/completions/_ekconf
```

Then restart your shell or run `compinit`.

### Aliases

```sh
alias kconf=ekconf
kubectl() {
  command ekconf exec --no-shell -- "$(command -v kubectl)" "$@"
}
```

## Configuration

```sh
# View current config
ekconf config list

# Enable macOS Keychain / Linux Keyring
ekconf config keychain=true

# Disable keychain
ekconf config keychain=false

# Colorize YAML output
ekconf config yaml.colorize=true
```

## How it works

Your kubeconfig data lives in a single encrypted file at `~/.ekube/config.enc`
(AES-256-GCM, key derived with Argon2id). A lightweight plaintext index at
`~/.ekube/config.yaml` holds only metadata (context names, namespaces) so
commands like `ls` and `use` never need your password.

## License

[MIT](LICENSE)
