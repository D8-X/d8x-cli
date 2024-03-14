# Updating your d8x services

When updating your d8x swarm and broker services, we recommend performing a
database backup before running the actual updates. If you make a backup, if
something goes wrong, you will still have the database that can be used to
restore the state of your previous deployments. Making a backup is also useful
when significant contract changes are introduced, for example changes in events
(around beginning of March in 2024). 

The following runbook will guide on how to perform a database backup and update
your services. 

Follow this runbook from top to bottom.

## Running the database backup

Refer to the [Database Backups](./README.md#database-backups) on more details
about how to perform a database backup.

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
┌──────────────────────────┐
│   ____     ___   __  __  │
│  |  _ \   ( _ )  \ \/ /  │
│  | | | |  / _ \   \  /   │
│  | |_| | | (_) |  /  \   │
│  |____/   \___/  /_/\_\  │
│                          │
└──────────────────────────┘
Updating swarm services...

Select swarm services to update

   [x] api
   [x] candles-pyth-client
   [x] candles-ws-server
   [x] history
   [x] referral
╭────────╮
│   OK   │
╰────────╯
Select broker-server services to update

   [x] broker
   [x] executorws
╭────────╮
│   OK   │
╰────────╯
Fetching image tags with sha hashes for service referral
Fetching image tags with sha hashes for service candles-pyth-client
Fetching image tags with sha hashes for service candles-ws-server
Fetching image tags with sha hashes for service api
Fetching image tags with sha hashes for service history
Image tags fetched for service ghcr.io/d8-x/d8x-candles-pyth-client
Image tags fetched for service ghcr.io/d8-x/d8x-trader-main
Image tags fetched for service ghcr.io/d8-x/d8x-trader-history
Image tags fetched for service ghcr.io/d8-x/d8x-candles-ws-server
Image tags fetched for service ghcr.io/d8-x/d8x-referral-system

Choose which image reference to update api service to

   [x] ghcr.io/d8-x/d8x-trader-main:main@sha256:ac06805f6be51a83e21dfa78d9d27ec425d169623f16ffa43484792a48d8a016
   [ ] ghcr.io/d8-x/d8x-trader-main:dev@sha256:2f306c1342d6f7aecc440fd8d841479cb63afa3e0e9b61dceb384a3118000928
   [ ] Enter image reference manually
╭────────╮
│   OK   │
╰────────╯
Service api will be updated to ghcr.io/d8-x/d8x-trader-main:main@sha256:ac06805f6be51a83e21dfa78d9d27ec425d169623f16ffa43484792a48d8a016

Choose which image reference to update candles-pyth-client service to

   [x] ghcr.io/d8-x/d8x-candles-pyth-client:main
   [ ] ghcr.io/d8-x/d8x-candles-pyth-client:dev@sha256:5373e33c382f72773d50e3ac7b47f739ca95b05ae6d0f11e1eac9ce800877f3a
   [ ] Enter image reference manually
╭────────╮
│   OK   │
╰────────╯
Service candles-pyth-client will be updated to ghcr.io/d8-x/d8x-candles-pyth-client:main

Choose which image reference to update candles-ws-server service to

   [x] ghcr.io/d8-x/d8x-candles-ws-server:main
   [ ] ghcr.io/d8-x/d8x-candles-ws-server:dev@sha256:081eb98ec939d0bfa7e58637fb541c985e36ab0092eca7dd5dc7396f1f5e89ef
   [ ] Enter image reference manually
╭────────╮
│   OK   │
╰────────╯
Service candles-ws-server will be updated to ghcr.io/d8-x/d8x-candles-ws-server:main

Choose which image reference to update history service to

   [x] ghcr.io/d8-x/d8x-trader-history:main@sha256:001704f5249a88cbd93272da705cd92c933837190653b1ad02e7b63add4a24df
   [ ] ghcr.io/d8-x/d8x-trader-history:dev@sha256:4e14361b0033ea1917971bd9293a017b0791edf34e7025a344cb091d223c7830
   [ ] Enter image reference manually
╭────────╮
│   OK   │
╰────────╯
Service history will be updated to ghcr.io/d8-x/d8x-trader-history:main@sha256:001704f5249a88cbd93272da705cd92c933837190653b1ad02e7b63add4a24df

Choose which image reference to update referral service to

   [x] ghcr.io/d8-x/d8x-referral-system:main@sha256:5c38fc8938f9cc85a4168386a684930a97cbe1bcdf06e51df1cf7f34b247cfcd
   [ ] ghcr.io/d8-x/d8x-referral-system:dev@sha256:cd1925abdcbb17fb063e17370bb3b376d5f85f395e9233873db7e05217098992
   [ ] Enter image reference manually
╭────────╮
│   OK   │
╰────────╯
Service referral will be updated to ghcr.io/d8-x/d8x-referral-system:main@sha256:5c38fc8938f9cc85a4168386a684930a97cbe1bcdf06e51df1cf7f34b247cfcd
Enter your referral payment executor private key:
> ****************************************************************

Wallet address of entered private key: 0xAc35CA4cC617CFf4143A1471151a904FE535F0c6
Is this the correct address?

╭─────────╮  ╭────────╮
│   yes   │  │   no   │
╰─────────╯  ╰────────╯

Pruning unused resources on worker servers...
Running docker prune on worker-1:
Deleted Containers:
aefad8dbc219ec4a17a9c5a86f3b56e00937ee2a8627db8a9908d377b5473dc4
fc1a21466bb6de3572ee6d6b8f8cde2dfb31d0d3bace9ea69d7924f0c60a0b27

Total reclaimed space: 0B
Total reclaimed space: 0B

Docker prune on worker 1 completed successfully
Running docker prune on worker-2:
Deleted Containers:
6e251b1f07c935d7b2ac61a0c6ae49fe44abf90bdcda8e3c3125bc54e0d96b9f
205a6f00d081dc3937e4cd3e5907dab50ca6b34eff49609a90e14a25f1e43e84

Total reclaimed space: 0B
Total reclaimed space: 0B

Docker prune on worker 2 completed successfully
Updating api to ghcr.io/d8-x/d8x-trader-main:main@sha256:ac06805f6be51a83e21dfa78d9d27ec425d169623f16ffa43484792a48d8a016
stack_api
overall progress: 2 out of 2 tasks
1/2: running
2/2: running
verify: Service converged
Service api updated successfully
<...>
```

