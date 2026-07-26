# d8x setup swarm-nginx

Deploys nginx configuration and SSL certificates to the manager node.

## Before this

Ensure DNS A records for all swarm domains (api, ws, history, candles) point to the manager IP. Certbot will fail if DNS is not configured.

## What it does

1. Selects environment
2. Fetches `nginx.conf`, `sites.conf`, `auth_check.conf`, `staging_origins.map` from infra repo
3. Substitutes `API_KEY_HERE` in `auth_check.conf` with `NGINX_API_KEY`
4. Installs certbot on manager
5. Deploys all config files via SSH
6. Sets nginx file limits
7. Tests nginx config
8. Reloads nginx
9. Optionally runs certbot for SSL certificates

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)
- `NGINX_API_KEY` (Bitwarden or .env)
- `SSH_KEY_{ENV}` (Bitwarden) or `SSH_KEY_PATH_{ENV}` (.env)
- `SERVER_PASSWORD_{ENV}` (Bitwarden or .env)

## After this

Proceed to `d8x setup broker-deploy`.
