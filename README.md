# D8X CLI

CLI tool for provisioning, deploying, and managing D8X trader backend, broker server, and nginx infrastructure.

## Installation

**Option A: Download from [releases](https://github.com/D8-X/d8x-cli/releases)**
```bash
tar -xzf d8x-linux-amd64.tar.gz
./d8x help
```

**Option B: Build from source** (requires Go 1.21+)
```bash
git clone https://github.com/D8-X/d8x-cli.git
cd d8x-cli
go build -o d8x ./main.go
./d8x help
```

**Dependencies:**

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

## Secrets

Secrets are stored in a shared Bitwarden note named `d8x-cli`. The CLI loads them automatically on each run.

```bash
bw login                                  # once
```

The CLI will prompt for the master password. To avoid re-entering it on every command, unlock once per terminal session:

```bash
export BW_SESSION=$(bw unlock --raw)
```

Secrets can also be passed via a `.env` file in the current working directory or CLI flags.

## Infrastructure

All infrastructure configs are in [D8-X/backend-nginx-infra-config](https://github.com/D8-X/backend-nginx-infra-config). Each folder is an environment (testnet, mainnet, etc.) containing nginx configs, terraform variables, hosts inventory, and auth settings. The CLI reads and writes to this repo via the GitHub API.

## New Deployment

See [docs/new-deployment.md](docs/new-deployment.md) for the full setup flow.

## Day-to-Day Operations

See [docs/maintenance.md](docs/maintenance.md) for updates, health checks, SSH access, backups, and troubleshooting.

## Quick Reference

| Command | Description |
|---------|-------------|
| `d8x update` | Update Docker service images |
| `d8x health` | Check all services and endpoints |
| `d8x setup staging-origins` | Manage nginx origin whitelist |
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

