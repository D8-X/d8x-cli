# d8x setup broker-nginx

Deploys nginx configuration and SSL certificate to the broker server.

## Before this

Ensure the DNS A record for the broker domain points to the broker server IP. Certbot will fail if DNS is not configured.

## What it does

1. Selects environment
2. Fetches `broker-nginx.conf` from infra repo
3. Substitutes `BROKER_PRIVATE_IP_HERE` with the broker private IP from `hosts.cfg`
4. Installs certbot on broker server
5. Deploys nginx config via SSH
6. Tests nginx config
7. Reloads nginx
8. Optionally runs certbot for SSL certificate

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)
- `SSH_KEY_{ENV}` (Bitwarden) or `SSH_KEY_PATH_{ENV}` (.env)
- `SERVER_PASSWORD_{ENV}` (Bitwarden or .env)

## After this

Deployment is complete. Run `d8x health` to verify all services.
