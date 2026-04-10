# d8x setup swarm-deploy

Deploys the D8X trader backend as a Docker Swarm stack.

## What it does

1. Selects environment
2. Generates swarm Redis password (20 chars) if not set
3. Saves Redis password to Bitwarden as `SWARM_REDIS_PW_{ENV}`
4. Sets up NFS shared storage on manager, mounts on workers
5. Copies `.env` and RPC configs to manager via SFTP
6. Creates Docker configs and volumes
7. Deploys `docker-swarm-stack.yml` on manager
8. Verifies ingress network

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)
- `SSH_KEY_{ENV}` (Bitwarden) or `SSH_KEY_PATH_{ENV}` (.env)
- `SERVER_PASSWORD_{ENV}` (Bitwarden or .env)

## After this

Proceed to `d8x setup swarm-nginx`.
