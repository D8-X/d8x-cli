# D8X CLI

CLI for provisioning, deploying, and managing D8X trader backend, broker server, and nginx infrastructure.

## Dependencies

macOS:
```bash
brew install ansible terraform bitwarden-cli
```

Linux (Ubuntu/Debian):
```bash
sudo apt install ansible
sudo snap install terraform --classic
sudo snap install bw
```

### Secrets

Secrets are stored in a shared Bitwarden note named `d8x-cli`. The CLI loads them automatically on each run.

```bash
bw login                                  # once
```

The CLI will prompt for the master password. To avoid re-entering it on every command, unlock once per terminal session:

```bash
export BW_SESSION=$(bw unlock --raw)
```

Secrets can also be passed via a `.env` file in the current working directory or CLI flags.

## Installation

**Quick install** (macOS and Linux):
```bash
curl -sL https://raw.githubusercontent.com/D8-X/d8x-cli/main/install.sh | bash
```

**macOS (Homebrew):**
```bash
brew tap D8-X/tap
brew install d8x
```

Or download manually from [releases](https://github.com/D8-X/d8x-cli/releases).

**Update:**
On macOS with Homebrew:
```bash
brew update && brew upgrade d8x     # Homebrew
```
For manual install or Linux:
```bash
curl -sL https://raw.githubusercontent.com/D8-X/d8x-cli/main/install.sh | bash  # re-run installer
```



## Infrastructure

The CLI reads and writes infrastructure configs from a GitHub repo via the API. Default repo is [D8-X/backend-nginx-infra-config](https://github.com/D8-X/backend-nginx-infra-config). To use a different repo, set `INFRA_REPO=your-org/your-repo` in `.env` or Bitwarden. If the repo is not accessible, the CLI will prompt for a different one.

See [docs/infra-repo.md](docs/infra-repo.md) for the repo structure and how to set up your own.

## New Deployment

See [docs/new-deployment.md](docs/new-deployment.md) for the full setup flow.

## Day-to-Day Operations

See [docs/maintenance.md](docs/maintenance.md) for updates, health checks, SSH access, backups, and troubleshooting.

## Quick Reference

**Setup commands:**

| Command | Alias | Description |
|---------|-------|-------------|
| `d8x setup new-env` | | Create a new environment in the infra repo |
| `d8x setup provision` | `prov` | Provision servers with Terraform |
| `d8x setup configure` | `config` | Configure servers with Ansible |
| | | |
| `d8x setup swarm-deploy` | `sd` | Deploy trader backend swarm |
| `d8x setup swarm-nginx` | `sn` | Deploy nginx + SSL for swarm |
| | | |
| `d8x setup broker-deploy` | | Deploy broker server |
| `d8x setup broker-nginx` | | Deploy nginx + SSL for broker |
| | | |
| `d8x setup metrics-deploy` | | Deploy Prometheus + Grafana |
| `d8x setup staging-origins` | `so` | Manage nginx origin whitelist |

**Operations:**

| Command | Description |
|---------|-------------|
| `d8x update` | Update Docker service images |
| `d8x health` | Check all services and endpoints |
| `d8x ssh manager\|broker\|worker-N` | SSH into a server |
| `d8x db-tunnel` | Create SSH tunnel to the database |
| `d8x backup-db` | Backup the database via SSH |
| `d8x grafana-tunnel` | Tunnel to Grafana dashboard |
| `d8x ip manager\|broker` | Show server IPs |
| `d8x fix-ingress` | Fix Docker Swarm ingress network |
| `d8x tf-destroy` | Destroy all provisioned servers (irreversible) |

## Troubleshooting

```bash
d8x ssh manager                        # SSH into swarm manager
docker service ls                       # list services
docker service logs stack_api -f        # check logs

d8x ssh broker                          # SSH into broker
cd broker && docker compose logs -f     # check broker logs
```

If swarm ingress is stuck (503 errors):
```bash
d8x fix-ingress
d8x setup swarm-deploy
```

