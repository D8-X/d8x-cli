# d8x setup broker-deploy

Deploys the broker server services (broker API, executor WebSocket, RPC proxy, Redis).

## Before this

Have the broker private key ready. The CLI will ask for it.

## What it does

1. Selects environment
2. Prompts for broker private key
3. Generates broker Redis password (16 chars)
4. Saves Redis password to Bitwarden as `BROKER_REDIS_PW_{ENV}`
5. Copies configs (`chainConfig.json`, `rpc.json`) to broker server via SFTP
6. Creates encrypted key volume for the broker private key
7. Runs `docker compose up` on broker server

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)
- `SSH_KEY_{ENV}` (Bitwarden) or `SSH_KEY_PATH_{ENV}` (.env)
- `SERVER_PASSWORD_{ENV}` (Bitwarden or .env)
- Broker private key (prompted)

## After this

Proceed to `d8x setup broker-nginx`.
