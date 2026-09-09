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
[`go-secretbox`](https://github.com/floatpane/go-secretbox), which provides
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

## Getting started

```sh
# Import your existing ~/.kube/config into the encrypted store
ekconf import

# Or add individual kubeconfig files
ekconf add ~/path/to/kubeconfig.yaml -n my-cluster
```

## Commands

| Command | Description |
|---|---|
| `ekconf add <path>` | Encrypt and merge a kubeconfig |
| `ekconf rename <old> <new>` | Rename a context (alias: `mv`) |
| `ekconf rm <name> [<name>...]` | Remove one or more contexts |
| `ekconf ls` | List all contexts (alphabetical, with certificate expiry) |
| `ekconf view <name>` | View a context's kubeconfig (redacted by default) |
| `ekconf view <name> --plain` | Include sensitive auth data |
| `ekconf use <name>` | Set the active context, warns if the client certificate expired |
| `ekconf ns <namespace>` | Set default namespace on the active context |
| `ekconf exec [<name>] -- <cmd>` | Run a command with decrypted KUBECONFIG |
| `ekconf rotate` | Re-encrypt with a new password |
| `ekconf migrate` | Migrate `config.enc` to the current encrypted format |
| `ekconf import [<name>...] [--force]` | Import contexts from `~/.kube/config` into the encrypted store |
| `ekconf eject [<name>...] [--merge] [--force]` | Decrypt and write or merge into `~/.kube/config` |
| `ekconf config list` | View configuration (colorized with` yaml.colorize=true`) |
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
# Respects shell aliases and functions. It's a drop-in replacement
# for your normal shell (e.g. alias kubectl=kubecolor, k=kubectl)
ekconf exec -- kubectl get pods
ekconf exec staging -- kubectl get pods
```

### Certificate expiry

`ekconf ls` shows the client certificate expiry in an EXPIRES column. Expired
dates are printed in yellow:

```
$ ekconf ls
  NAME          NAMESPACE    EXPIRES
  demo                       2027-09-10 (in 1y)
* prod          production   2025-03-01 (expired)
  soon          kube-system  2026-09-18 (in 9d)
  token-only
```

Dates are in your machine's local timezone, with the remaining lifetime in
parentheses. The NAMESPACE and EXPIRES columns are dropped when no context has
a value for them.

`ekconf use` warns when the context you switch to has an expired certificate:

```
$ ekconf use prod
Switched to context 'prod'
Warning: client certificate for 'prod' expired on 2025-03-01 14:22:05 UTC
```

The expiry date is read from `~/.ekube/config.yaml`, so no password is needed. It
is recorded there whenever a command decrypts the store: `add`, `import`, `rename`,
`rm`, `view` and `exec`. Contexts added before this feature get their date on the
next such command. Contexts that authenticate with a token or an exec plugin have
no client certificate, so no warning is shown for them.

The first `ekconf add` or `ekconf import` creates the store, and that password
must be at least 12 characters. Interactive prompts ask for confirmation, since
a typo there is unrecoverable. An existing store keeps whatever password it has.

### Release integrity

`ekconf update` refuses to install a release unless the `checksums.txt` carries a
valid ed25519 signature from the project signing key, which is compiled into the
binary. It also refuses to install a release that is not newer than the version
you are running. Pass `--force` to install an older release on purpose.

Releases also carry GitHub build provenance. To check a downloaded archive:

```sh
gh attestation verify ekconf_1.2.0_darwin_arm64.tar.gz --repo eznix86/ekconf
```

### Password resolution

Checked in order:

1. `--password=<value>` flag (visible in the process table and shell history, prefer `--password-stdin`)
2. `--password-stdin` flag
3. `EKCONF_PASSWORD` environment variable
4. System keychain (if `keychain=true`)
5. Interactive prompt

Enable keychain storage:

```sh
ekconf config keychain=true
```

> **Linux**: requires `libsecret` and a Secret Service provider
> (`gnome-keyring`, `kwallet`, etc.). Install with your package manager:
> `apt install libsecret-1-0 gnome-keyring` or `dnf install libsecret gnome-keyring`.

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

# Plain kconf (original) renamed to pkconf so both encrypted and
# unencrypted/ejected kubeconfigs can be managed side by side
alias pkconf="/opt/homebrew/bin/kconf"

# ekconf exec uses a shell by default, so aliases and functions
# defined in your shell config are available to the command.
# Add --no-shell to run the binary directly and bypass aliases.
alias helm="ekconf exec -- helm"
alias helmfile="ekconf exec -- helmfile"
alias ktop="ekconf exec -- ktop"

# --no-shell bypasses shell aliases and functions (runs the binary directly)
alias kubectl="ekconf exec --no-shell -- kubectl"

# Works with kubecolor, k9s, custom scripts, or any kubectl wrapper
alias k=kubectl
alias kubecolor="ekconf exec --no-shell -- kubecolor"
stern() {
  command ekconf exec -- "$(command -v stern)" "$@"
}

# Custom kubectl wrapper scripts also work
my-kubectl() {
  command ekconf exec -- /usr/bin/custom-kubectl.py "$@"
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

# Colorize YAML output (view, config list)
ekconf config yaml.colorize=true
```

## How it works

Your kubeconfig data lives in a single encrypted file at `~/.ekube/config.enc`
(AES-256-GCM, key derived with Argon2id). An index at
`~/.ekube/config.yaml` holds only metadata (context names, namespaces) so
commands like `ls` and `use` never need your password.

### Threat model

Read this before deciding what `ekconf` is worth to you.

#### What it protects against

**Copies of your home directory that end up elsewhere.** Time Machine, a home
directory synced to iCloud or Dropbox, an rsync to a NAS, an archive you send a
colleague. Those copies outlive the machine and land on systems with different
access rules. `config.enc` is ciphertext in all of them.

**A lost or stolen laptop that is powered off.** Full-disk encryption covers
this too, when it is enabled and the machine is actually off rather than
suspended.

**Accidental disclosure.** A screen share, a pasted support bundle, a recorded
demo, a `cat` in a terminal someone else is watching.

**The exposure window.** A plaintext `~/.kube/config` is readable every second
of every day, and many tools open it. With `ekconf` the decrypted form exists
only for the duration of a single command.

#### What it does not protect against

**Anything running as your user.** A malicious dependency, a compromised editor
extension, a rogue postinstall script. Such a process does not need to break the
encryption. It can read the OS keychain when `keychain=true`, read the temp file
while `ekconf exec` is running, log the password prompt, or replace the `ekconf`
binary on your `PATH`.

Encryption at rest cannot defend a machine that is already compromised. If yours
is, assume every credential in the store is compromised too, and rotate them.

The defence that does work there is credentials that expire on their own: OIDC,
or your cloud provider's exec plugin. A stolen short-lived token is worthless in
an hour. `ekconf` changes where your credentials come to rest. It does not make
a compromised machine safe.

#### Offline brute force

`config.enc` is protected by Argon2id (64 MiB, 3 iterations), which costs
roughly 0.5s per attempt. There is no lockout or attempt limit: anyone who
obtains the file can brute-force it offline at that cost per guess. Your
password is the only thing standing between an attacker and your cluster
credentials.

Creating the store and running `ekconf rotate` both require at least 12
characters. A long passphrase is worth more than a short complex one. Enabling
`keychain=true` keeps the password out of your shell history and scripts, at the
cost of making it readable by any process running as you.

#### Password in memory

The password is held as a byte slice and zeroed as soon as each command is done
with it. The keychain path is the exception: `go-keyring` exposes a string-only
API, so on `keychain=true` the password also lands in immutable Go strings that
cannot be zeroed and persist until garbage collection. This only matters against
an attacker who can already read the process heap, who has easier options
available anyway.

##### `ekconf import [<name>...] [--force]`

Migrate from a plaintext `~/.kube/config` into the encrypted store. With no
names, every context is imported.

```sh
ekconf import                # import every context, keep the source
ekconf import prod           # import only prod
ekconf import prod staging   # import both
ekconf import --force        # import everything and remove ~/.kube/config
```

`--force` cannot be combined with names, because removing `~/.kube/config`
would delete the contexts you did not import.

##### `ekconf eject [<name>...] [--merge] [--force]`

Export contexts from the encrypted store back to plaintext `~/.kube/config`.

```sh
ekconf eject                                  # all contexts, overwrite prompt
ekconf eject prod                             # single context, overwrite prompt
ekconf eject staging prod --merge             # merge into existing config
ekconf eject staging prod --merge --force     # merge and replace conflicts
```

## License

[MIT](LICENSE)
