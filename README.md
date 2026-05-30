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

## Description

`ekconf` lets you add, remove, list, view, and switch between kubeconfig contexts —
just like `kconf`. The difference: all kubeconfig data is encrypted on disk.

Your configs live in `~/.ekube/config.enc` (AES-256-GCM with Argon2id key derivation).
A lightweight plaintext index at `~/.ekube/config.yaml` holds just enough metadata
(context names, namespace) to answer queries without a password.

## Installation

```sh
go install github.com/eznix86/ekconf@latest
```

Or build from source:

```sh
git clone https://github.com/eznix86/ekconf
cd ekconf
go install .
```

## Usage

```sh
# Add a kubeconfig
ekconf add ~/.kube/config
ekconf add /path/to/some/other.conf -n my-cluster

# Remove a context
ekconf rm my-cluster

# List all contexts
ekconf ls

# View a single context's kubeconfig
ekconf view my-cluster

# Switch to a context
ekconf use my-cluster

# Set default namespace
ekconf ns my-namespace

# Run a command with decrypted config
ekconf exec -- kubectl get pods
ekconf exec staging -- kubectl get pods

# Re-encrypt with a new password
ekconf rotate

# Decrypt and write back to ~/.kube/config
ekconf eject
```

### Shell completion

For a one-shot zsh session:

```sh
source <(ekconf completion zsh)
```

For persistent completion, install the generated script:

```sh
ekconf completion zsh > ~/.zsh/completions/_ekconf
```

Then restart your shell or rerun `compinit`.

### Aliases

Add these to your shell rc file:

```sh
alias kconf=ekconf
alias kubectl="ekconf exec -- kubectl"
```

## Password Resolution

`ekconf` checks the following in order:

1. `--password=<value>` flag
2. `--password-stdin` flag
3. `EKCONF_PASSWORD` environment variable
4. System keychain (if `keychain=true` in config)
5. Interactive prompt

Enable keychain storage:

```sh
ekconf config keychain=true
```

## Why?

`kconf` is great — but it stores your kubeconfigs in plaintext. If you manage
production clusters, your `~/.kube/config` contains credentials that can unlock
your infrastructure. `ekconf` keeps those credentials encrypted at rest while
preserving the same workflow.

## License

[MIT](LICENSE)
