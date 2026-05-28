# d8x setup swarm-deploy

Deploys the D8X trader backend as a Docker Swarm stack.

## What it does

1. Selects environment
2. Generates swarm Redis password (20 chars) if not set
3. Saves Redis password to Bitwarden as `SWARM_REDIS_PW_{ENV}`
4. Copies `.env` and RPC configs to manager via SFTP
5. Creates Docker configs
6. Deploys `docker-swarm-stack.yml` on manager
7. Verifies ingress network

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)
- `SSH_KEY_{ENV}` (Bitwarden) or `SSH_KEY_PATH_{ENV}` (.env)
- `SERVER_PASSWORD_{ENV}` (Bitwarden or .env)

## After this

Proceed to `d8x setup swarm-nginx`.
