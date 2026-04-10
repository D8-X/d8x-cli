# d8x setup configure

Configures all provisioned servers with Ansible.

## What it does

1. Selects environment
2. Generates server password (16 chars) if not set
3. Saves password to Bitwarden as `SERVER_PASSWORD_{ENV}`
4. Runs Ansible playbook: installs Docker, sets up users, SSH keys, firewall

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)
- `SSH_KEY_{ENV}` (Bitwarden) or `SSH_KEY_PATH_{ENV}` (.env)

## After this

Proceed to `d8x setup swarm-deploy`.
