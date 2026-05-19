# Database

Only the `history` service uses Postgres (one DB, default name `history`). The other services use Redis or nothing.

## Where it runs

| Provider | Postgres |
| --- | --- |
| AWS | Managed RDS, provisioned by `setup provision` |
| Linode | Container in the swarm, deployed by `setup swarm-deploy` |

## Credentials (Bitwarden)

| Field | Set by |
| --- | --- |
| `AWS_RDS_HOST/PORT/USER/PASSWORD_<ENV>` | `setup provision` (AWS) |
| `AWS_RDS_DB_NAME_<ENV>` | `setup provision` (AWS, defaults to `history`) |
| `DATABASE_DSN_<ENV>` | `setup provision` (AWS) or `setup swarm-deploy` (Linode) |

The DSN is `postgres://user:password@host:port/<dbname>?sslmode=…`. Trader-backend reads it from its `.env` as `DATABASE_DSN`; `db-tunnel` and `backup-db` read `DATABASE_DSN_<ENV>` from Bitwarden.

## `history` DB creation

- **AWS**: at the end of `setup provision` the CLI prompts to auto-create the DB. If it fails, run `psql "$DATABASE_DSN"` + `CREATE DATABASE history;` from the manager (`d8x ssh manager`), or rerun `setup provision`.
- **Linode**: the container creates its DB on first boot.

Schema migrations are run by the `history` container itself on startup.

## Operations

```bash
d8x db-tunnel [LOCAL_PORT]         # tunnel to Postgres (default 5432)
d8x backup-db [--output-dir DIR]   # pg_dump backup
```

Both resolve the DSN from Bitwarden (`DATABASE_DSN_<ENV>`). Tunnels run in the foreground; `Ctrl+C` to stop.

## Troubleshooting

- **`db-tunnel` exits immediately**: `DATABASE_DSN_<ENV>` missing in Bitwarden. Re-run `setup provision` (AWS) or `setup swarm-deploy` (Linode).
- **Wrong env**: DSN is env-suffixed. Confirm the selected env before running ops commands.
