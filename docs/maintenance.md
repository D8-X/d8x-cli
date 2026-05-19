# Maintenance

## Day-to-day commands

| Command | What it does |
| --- | --- |
| `d8x update` | Select swarm and/or broker services and roll them to the latest image |
| `d8x health` | Check HTTP endpoints, swarm services, broker RPC proxy |
| `d8x setup staging-origins` | Add/remove whitelisted staging origins; commits to infra repo and deploys |
| `d8x setup rpc` | Add/remove RPC URLs; route to `api`, `history`, or both. See [setup-rpc.md](setup-rpc.md) |
| `d8x setup swarm-nginx` | Redeploy swarm nginx + SSL |
| `d8x setup broker-nginx` | Redeploy broker nginx + SSL |
| `d8x ssh manager\|broker\|worker-N` | Open an SSH session |
| `d8x ip manager\|broker` | Print the node's public IP |
| `d8x grafana-tunnel [PORT]` | Tunnel to Grafana (default local port 8080) |
| `d8x db-tunnel [PORT]` | Tunnel to Postgres (default local port 5432). See [database.md](database.md) |
| `d8x backup-db [--output-dir DIR]` | `pg_dump` backup. See [database.md](database.md) |
| `d8x fix-ingress` | Reset the swarm ingress network if requests are 503ing |

Tunnels run in the foreground; `Ctrl+C` to stop.

## Troubleshooting

Logs and service status (on the manager):

```bash
d8x ssh manager
docker service ls
docker service logs stack_api -f       # or stack_history, stack_candles, ...
```

Broker logs (on the broker host):

```bash
d8x ssh broker
cd broker && docker compose logs -f
```

If swarm ingress is stuck (503s on the api/ws endpoints):

```bash
d8x fix-ingress
d8x setup swarm-deploy
```
