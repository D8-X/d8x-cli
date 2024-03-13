# Updating your d8x services

When updating your d8x swarm and broker services, we recommend performing a
database backup before running the actual updates. If you make a backup, if something goes wrong, you will still have the database that can be used to restore the state of your previous deployments. Making a backup is also useful when significant contract changes are introduced, for example changes in events (around beginning of March in 2024). 

The following runbook will guide how to perform a database backup and update your services. 

Follow this runbook from top to bottom.

## Running the database backup

Refer to the [Database Backups](./README.md#database-backups) on more details about how to perform a database backup.

First, you should run the `d8x backup-db` command:

```bash
d8x backup-db
```

Will result with output similar to the following:
```bash
┌──────────────────────────┐
│   ____     ___   __  __  │
│  |  _ \   ( _ )  \ \/ /  │
│  | | | |  / _ \   \  /   │
│  | |_| | | (_) |  /  \   │
│  |____/   \___/  /_/\_\  │
│                          │
└──────────────────────────┘
Backing up database...

Determining postgres version
Postgres server at lin-55017-35110-pgsql-primary-private.servers.linodedb.net version: 14.6
Ensuring pg_dump is installed on manager server (postgresql-client-16)
Creating database testing_db backup
Backup file size: 0.023752 MB
Database testing_db backup file was downloaded and copied to /home/mantas/work/d8x-cli/build/backup-d8x-cluster-testing-linode-2024-03-12-17-36-36.dump.sql
Removing backup file from server
```

Make sure you securely store the backup file.

## Running the update for specific or all services

Refer to [Readme](./README.md) for more information about the `d8x update` command.

Run `d8x update` and select the service(s) you want to update. Depending on
which services are updated, you might be asked to enter private keys or other
information.

```bash
d8x update
```

```bash
Updating swarm services...

Select swarm services to update

   [x] api
   [ ] candles-pyth-client
   [ ] candles-ws-server
   [x] history
   [ ] referral
╭────────╮
│   OK   │
╰────────╯
Select broker-server services to update

   [ ] broker
   [ ] executorws
╭────────╮
│   OK   │
╰────────╯
Pruning unused resources on worker servers...
Running docker prune on worker-1:
<...>
Updating service api
Choose which image reference to update to

   [ ] ghcr.io/d8-x/d8x-trader-main:main@sha256:ac06805f6be51a83e21dfa78d9d27ec425d169623f16ffa43484792a48d8a016
   [x] ghcr.io/d8-x/d8x-trader-main:dev@sha256:2f306c1342d6f7aecc440fd8d841479cb63afa3e0e9b61dceb384a3118000928
   [ ] Enter image reference manually
╭────────╮
│   OK   │
╰────────╯
Using image: ghcr.io/d8-x/d8x-trader-main:dev@sha256:2f306c1342d6f7aecc440fd8d841479cb63afa3e0e9b61dceb384a3118000928
Updating api to ghcr.io/d8-x/d8x-trader-main:dev@sha256:2f306c1342d6f7aecc440fd8d841479cb63afa3e0e9b61dceb384a3118000928
stack_api
overall progress: 2 out of 2 tasks
1/2: running
2/2: running
verify: Service converged
Service api updated successfully
<...>
```

