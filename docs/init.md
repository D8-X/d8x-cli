# d8x init

Check that `terraform` and `ansible` are installed; offer to install missing ones on Linux.

```bash
d8x init
```

## What you'll see

1. The CLI checks `PATH` for `terraform` and for the ansible toolchain (`ansible` + `ansible-playbook`). Two status lines are printed: `Terraform found!` / `Ansible found!` (or the matching `not found` lines).
2. **macOS**: if anything is missing, the command exits with a Homebrew install hint, for example `Install via Homebrew (e.g. "brew install terraform ansible") and retry`.
3. **Linux**: if anything is missing, a multi-select prompt lets you pick which to install automatically. Pick at least one to proceed; pick nothing and the command exits with a clear "missing dependencies" error.
4. After install attempts, the CLI re-checks `PATH`. If anything is still missing, the command exits with `still missing after install attempt: ...`.

## Install paths used (Linux)

| Package manager | terraform install |
| --- | --- |
| `dnf` (Fedora) | `dnf config-manager --add-repo` + `dnf install terraform` |
| `yum` (RHEL / CentOS) | `yum-config-manager --add-repo` + `yum install terraform` |
| `apt` (Debian / Ubuntu) | Adds the Hashicorp apt repo, installs `lsb-release` if missing, then `apt install -y terraform` |
| none of the above | command exits with a manual-install link |

Ansible is installed via `pipx install --include-deps ansible`, after ensuring `pipx` and `python3` are available. Required ansible-galaxy collections (`community.docker`, `ansible.posix`, `community.general`) are installed afterwards.

## Requirements

- `sudo` access (the install scripts run with `sudo bash` for system-level packages).
- `python3` already on PATH if ansible needs to be installed.
- An internet connection (Hashicorp / pip / ansible-galaxy mirrors).

## When things go wrong

- **`no supported package manager found`**: install terraform manually from https://developer.hashicorp.com/terraform/downloads, then rerun `d8x init`.
- **`python3 was not found in PATH`**: install python3 from your distribution first, then rerun.
- **Install command fails midway**: the error wraps the underlying terraform/ansible installer output. Fix the reported issue (network, repo key, sudo password, etc.) and rerun.
- **Already installed but version is too old**: `d8x init` only checks presence, not version. Upgrade via your package manager directly.

## Notes

- This command does not create any persistent files. The temporary install script lives in the OS temp dir and is deleted at function exit.
- Safe to run repeatedly. Existing `terraform`, `pipx`, and `ansible` are detected and skipped. The ansible-galaxy collections (`community.docker`, `ansible.posix`, `community.general`) are reinstalled on every run, which is a no-op when they're already up to date.
