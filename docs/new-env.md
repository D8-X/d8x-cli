# d8x setup new-env

Creates a new environment in the infra repo.

## What it does

1. Prompts for environment name, chain ID, domain, provider, region, worker count
2. Pushes to the infra repo:
   - `config.json` — chain ID and provider
   - `terraform.tfvars` — server specs (region, workers, sizes)
   - `nginx.conf` — main nginx config with origin map and Cloudflare IPs
   - `sites.conf` — server blocks for api, ws, history, candles
   - `broker-nginx.conf` — broker server block with `/rpc` proxy
   - `auth_check.conf` — origin/API key whitelist with `API_KEY_HERE` placeholder
   - `staging_origins.map` — empty

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)

## After this

Proceed to `d8x setup provision`.

Created the env by mistake? `d8x setup rm-env` deletes the directory from the infra repo (see [rm-env.md](rm-env.md)).
