# Maintenance

## Updates

```bash
d8x update
```

Select swarm and/or broker services to update to the latest image version.

## Health Check

```bash
d8x health
```

Checks all HTTP endpoints, Docker swarm services, and broker RPC proxy.

## Staging Origins

```bash
d8x setup staging-origins
```

Add or remove whitelisted staging origins. Changes are committed to the infra repo and deployed to the server.

## SSH Access

```bash
d8x ssh manager
d8x ssh broker
d8x ssh worker-1
```

## Database

```bash
d8x db-tunnel                      # SSH tunnel to database on local port 5432
d8x db-tunnel 5433                 # use custom local port
d8x backup-db                      # backup to current directory
d8x backup-db --output-dir ./bak   # backup to specific directory
```

Tunnels run in the foreground. `Ctrl+C` to stop.

## Monitoring

```bash
d8x grafana-tunnel     # tunnel to Grafana on port 8080
```

## Nginx Reconfiguration

```bash
d8x setup swarm-nginx  # redeploy swarm nginx + SSL
d8x setup broker-nginx # redeploy broker nginx + SSL
```

## Troubleshooting

SSH into the server and check logs:

```bash
d8x ssh manager
docker service ls
docker service logs stack_api -f
```

For broker:

```bash
d8x ssh broker
cd broker && docker compose logs -f
```

If swarm ingress is stuck (503 errors):

```bash
d8x fix-ingress
d8x setup swarm-deploy
```
