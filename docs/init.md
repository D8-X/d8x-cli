# d8x init

Check that `terraform` and `ansible` are installed; offer to install missing ones on Linux.

```bash
d8x init
```

## What you'll see

1. If you launched `d8x init` as root (e.g. via `sudo`), the CLI prints a warning and asks you to confirm before continuing. `pipx`, `ansible`, and `ansible-galaxy` are user-scoped tools and installing them under `/root` makes them invisible to later commands run as your normal user. The recommended action is to abort and rerun without `sudo`.
2. The CLI checks `PATH` for `terraform` and for the ansible toolchain (`ansible` + `ansible-playbook` + `ansible-galaxy`) and prints a found/not-found line for each.
3. **macOS**: if anything is missing, the command exits with a Homebrew install hint.
4. **Linux**: a multi-select prompt lets you pick what to install. Picking nothing exits with a clear "missing dependencies" error.
5. After install, the CLI re-checks `PATH` (with `~/.local/bin` auto-added so freshly pipx-installed tools show up). Anything still missing causes a `still missing after install attempt` exit.

## How install works (Linux)

- **terraform**: installed via your distro's package manager (`dnf`, `yum`, or `apt`) after adding the Hashicorp repo. Pre-existing Hashicorp repo files are detected and never overwritten. If none of the supported package managers is found, the command exits with a manual-install link.
- **ansible**: installed via `pipx`. If `pipx` is missing, the CLI first tries the system package manager, then falls back to `pip install --user --break-system-packages` (handles PEP 668 on recent distros). After ansible is installed, `passlib` is injected into the same pipx venv (needed by the d8x playbook's `password_hash` filter).
- **ansible-galaxy collections** (`community.docker`, `ansible.posix`, `community.general`) are then installed; this step is idempotent.

## Requirements

- Run as your normal user, **not** root. The terraform install handles its own elevation via `sudo` per command; tools that need to live in your home (pipx, ansible, collections) are installed without sudo.
- `sudo` on PATH (terraform install and pipx-via-distro install need it).
- `python3` on PATH if ansible needs to be installed.
- An internet connection.

## When things go wrong

- **`sudo not found in PATH`**: install sudo or install terraform manually, then rerun.
- **`no supported package manager found`**: install terraform manually from https://developer.hashicorp.com/terraform/downloads, then rerun.
- **`python3 was not found in PATH`**: install python3 first.
- **Install fails midway**: the error wraps the underlying installer output. Fix the reported issue (network, sudo password, etc.) and rerun.
- **`pipx inject ansible passlib` fails**: usually means ansible was installed by some other method (e.g. apt). Remove that ansible and let `d8x init` reinstall via pipx, or install passlib separately into ansible's Python environment.
- **Hashicorp repo file exists but points to the wrong place**: the install script does not overwrite it. Delete the stale `hashicorp.repo` / `hashicorp.list` / `hashicorp-archive-keyring.gpg` and rerun.

## Notes

- No persistent local files are created; the install script lives in the OS temp dir and is removed on exit.
- Safe to run repeatedly. Existing `terraform`, `pipx`, and `ansible` are skipped. The ansible-galaxy collection install reruns each time but is a no-op when they're up to date.
- `d8x init` only checks presence, not version. Upgrade via your package manager directly when needed.
