# D8X CLI

D8X CLI helps you to easiliy spin up d8x-trader-backend and
other d8x broker services.

Setup includes provisioning resources on supported cloud providers, configuring
servers, deploying swarm cluster and individual services.


## Pre-Flight Checklist

- Create a Quicknode account that allows for several nodes (choose the 49$/month plan) using [this link](https://www.quicknode.com/signup). In a second step, we suggest you onboard Quicknode through us. Any potential kickback that we get will directly go to you if you subscribe   using [this template](https://quiknode.typeform.com/to/efkXcHuc).
- Create 5 RPC endpoints on Quicknode for Polygon zkEVM testnet (chain id 1442) and 5 RPC endpoints for Polygon zkEVM mainnet (chain id 1101)
- Create 3 private keys, one we call "broker key" the other one we call "executor key", and the last one we call "payment key"
- Fund the "broker" and "executor" with ETH on testnet (1442) and mainnet (1101), send around $20 worth of ETH to the broker, around $100 for the executor
- Decide on whether you will deploy the backend on Linode or AWS.
  - If on Linode, create an API token. On the Linode website after logging in, click on your profile name (top right) and select [API Tokens](https://cloud.linode.com/profile/tokens), click on 'create personal access token' and follow the instructions. You will need the API key in the CLI.
  - If on AWS, create a new dedicated AWS account. Find the IAM (Identity and Access Management) service and navigate to "Create User". Fill in your user name and click next. In "Set permissions" step, select "Attach policies directly" search and attach `AmazonEC2FullAccess` and `AmazonRDSFullAccess`. Once you created your user, click on it to go to user's overview. In "Summary" click to "Create access key". Select "Local code" use case. Enter some informative description about this key like "Access key to run d8x-cli". Copy and securely store the **Access key** and **Secret access key** which are displayed at the end of this process. You will need to enter these values when running CLI for aws deployment.
- Linode users need an external database cluster. 
  <details>
    <summary>We recommend you create a free PostgreSQL cluster on <a href='https://aiven.io/postgresql'>Aiven</a></summary>
    
    - sign up a user on Aiven <a href='https://console.aiven.io/signup'>here</a>. Choose the 'business' option.
    - you will be forwarded to the "Services" page. Choose PosgreSQL, then 'free plan' and choose a region close to the region you plan to deploy your hardware, as the name choose anything you like (d8xcluster if unsure), and click "create free service"
    - You will see connection details. Click "skip this step"
    - You will be able to restrict IP addresses on the next screen: click skip this step (we can restrict later)
    - Now you have a database cluster available. Navigate on the left bar to 'Databases' and click on the right upper corner on 'Create database'. Create one database 'd8x_1442' (add database). Create another database called 'd8x_1101'. You should see now three databases listed: d8x_1442, d8x_1101, and defaultdb.
    - Navigate to 'Overview' on the left sidebar. You can see 'Service URI'. This will be the "DSN-string" that yo will have to provide to the CLI, replacing 'defaultdb' with 'd8x_1101' for mainnet and 'd8x_1442' for testnet, for example:
      `postgres://avnadmin:AVNS_TOAs8gaRaajKBWBckzsq@d8xcluster-pudgybear-5e36.a.aivencloud.com:11437/defaultdb?sslmode=require` -> you replace defaultdb by d8x_1101: `postgres://avnadmin:AVNS_TOAs8gasa#jKBWBckzsq@d8xcluster-pudgybear-5e36.a.aivencloud.com:11437/d8x_1101?sslmode=require` to get the DNS string that you will be prompted for when setting up mainnet

  </details>
- Get access to your domain name server, you will have to create A-name entries once you have the IP addresses of the servers available
  Typically a config entry looks something like this:

  |        Hostname        | Type |  TTL   |      Data       |
  | :--------------------: | :--: | :----: | :-------------: |
  | api.dev.yourdomain.com |  A   | 1 hour | 139.144.112.122 |
- Decide what default broker fee you will charge the traders
- You can use Linux (or a Linux Virtual Machine) or Mac to run the CLI. Install the CLI as directed below.

# CLI Installation

When using Linux, head to [releases](https://github.com/D8-X/d8x-cli/releases), download and
extract the d8x binary.

To run D8X-CLI on MacOS you will need to build it from source.

Building from source requires you to have Go 1.21+ installed on your machine.

## Using make

```bash
make install
cp ./d8x /usr/local/bin
```

### When using Mac
Install ansible, terraform, and go
```
brew update
brew install ansible
brew install terraform
brew install go
```

### Building From Source
* ensure you have go >=1.20, for example
  ```
  $ go version
    go version go1.21.4 linux/amd64
  ```
* checkout the repository into a local folder of your choice and navigate into the folder d8x-cli
* build
```bash
go build -o d8x ./main.go
```
* now you have a binary file 'd8x'.
* create a folder somewhere and copy the binary file to this folder, for example:
  ```
  mkdir ~/d8x-deployment
  mv ./d8x ~/d8x-deployment
  ```

# Starting D8X Setup
* copy the binary into a 'deployment folder' of your choice and navigate to this folder, for example:
  ```
  cd ~/d8x-deployment
  mkdir ./deploy
  cd deploy
  ```
* Now you can start the CLI from folder ~/d8x-deployment/deploy (if the binary is in ~/d8x-deployment/), for example:
  ```
  ../d8x help
  ```
* Run the setup with
```bash
../d8x setup
```


# Post-Flight Checklist
- Consider running your own Hermes price service to reliably stream prices: [details](https://docs.pyth.network/documentation/pythnet-price-feeds/hermes). Feel free to contact us for recommendations.
The service endpoint will have to be added to the configuration file (variable priceServiceWSEndpoints of the candles-service or prompt in CLI)
- When using an external database with Linode: consider whitelisting the IP addresses of the manager/nodes and blocking other IPs
- Setup a monitoring of the referral executor wallet ETH funds. The referral address needs ETH to pay for gas when executing referral payments.
- Regularly backup the database tables relevant for referrals, in particular: `referral_chain`, `referral_code`, `referral_code_usage`. If you reset/erase the database for some reason, without backing up and restoring these tables, referrers/traders will lose their referral codes and would no longer be rebated.
- Consider copying, compressing and encrypting the folder you deployed from and sharing the resulting zip file among the relevant personal within your company. The file contains important credentials that allow to access the deployment servers and changing/updating/maintaining the system.

# Configuration Files

Configuration files are key to setup D8X Perpetuals Backend. Although the CLI guides you through the config,
here you can find out details about the configuration
[README](README_CONFIG.md).

# Usage Of The D8X CLI Tool

Here are more details and options on the CLI tool. As noted above, running `d8x setup` is the only
command you need for the initial setup.

<details>
  <summary><h2>Setup</h2></summary>
  Performing a complete servers provisioning + configuration and services
deployment can be done with the `setup` command:

```bash
d8x setup
```

The `setup` command is an aggregate of multiple individual subcommands such as
`provision`, `configure`, `swarm-deploy`, etc. Setup command will walk you
through entire process of provisioning servers and deploying all the services.
See `d8x setup --help` for more information and available `setup` subcommands.



<h3>Provisioning and Configuration</h3>

Setup will start with provisioning servers with terraform and configuring them
with ansible. Depending on your selected server provider, you will need to
provide API tokens, access keys and other necessary information.

After provisioning and configuration is done a couple of files will be created
in your current working directory, such as:

- hosts.cfg - ansible inventory file
- id_ed25519 - ssh key used to access servers (added to each provisioned server)
- id_ed25519.pub - public key of id_ed25519
- password.txt - default user password on all servers
- redis_broker_password.txt - password for redis used on the broker server
- aws_rds_postgres.txt - aws postgres instance credentials (only for AWS provider)
  - manager_ssh_jump.conf - ssh config file for manager server to be used as jump host (only for AWS provider)

Do not delete these files, as they are to establish connections to your servers
when running individual `setup` subcommands.

For Linode - you need to provision a database cluser and instance by
yourself and provide the linode database instance id in the setup process. This
step is manual due to the fact that database instance provisioning very slow on
linode and usually takes around 30 minutes.

**Note** that at the time of writing this documentation (2023 October), Linode
has disabled option to create new managed databases. Therefore, you might need
to use external Postgres database which you can either provision yourself or use
an external database provider. Don't forget to update your database's security
policies to allow access from all of the provisioned servers ip addresses. You
can find more information how to do the external database setup in [this document](./docs/AIVEN_SETUP.md).

For AWS provider - RDS Postgres instance will be provisioned automatically. You
will be able to automatically create new databases for history and referral
services. Database credentials will be stored in `aws_rds_postgres.txt` file.
Note that RDS instance is provisioned in a private subnet, therefore you will
need to use manager node as a jump host in order to access it from your local or
other machine.

For example, to establish a ssh tunnel on port 5433 to your RDS instance, you
can run the following command:

```bash
ssh -F ./manager_ssh_jump.conf jump_host -L 5433:<YOUR_RDS_HOSTNAME_HERE>:5432 -v -N
```

</details>



<details>
  <summary><h2>Broker Server</h2></summary>

Upon selecting broker-server provisioning, deployment and nginx + certbot configuration will be performed.
When you select to configure SSL (certbot setup), you need to set up your DNS "A"
records of your provided domains to point to your broker-server public ip
address. This ip address will be displayed to you in the setup process, or you
can find it in `hosts.cfg` file or in your server provider's dashboard.

Follow the instructions in the setup process on which configuration files you
should modify before each step.

To run only broker deployment:

```bash
d8x setup broker-deploy
```

To run only broker nginx+certbot setup:

```bash
d8x setup broker-nginx
```
</details>

<details>
  <summary><h2>Trader backend (Swarm)</h2></summary>

Trader backend docker swarm deployment and nginx + certbot setup is analogous to
broker-server deployment, but involves slightly more configuration files. Make
sure to follow the setup instructions and modify the configuration files as well
as `.env` file.

Nginx + certbot setup is completely analogous to the broker-server setup, but
involves more domains for services.

To run only the trader backend swarm deployment:

```bash
d8x setup swarm-deploy
```

Command `d8x setup swarm-deploy` can be run multiple times. For example if you
have modified any configuration or .env files, you can simply rerun `d8x setup
swarm-deploy` to redeploy the services. This will remove existing services and
redeploy them via the manager node. Note that this will result in some downtime.

To run only trader backend swarm nginx+certbot setup:

```bash
d8x setup swarm-nginx
```
</details>

<details>
  <summary><h2>Teardown</h2></summary>

If you wish to completely remove any provisioned resources, you can do so by
running `tf-destroy` command.

```bash
d8x tf-destroy
```

**Note that this action is irreversible**

</details>


<details>
  <summary><h2>Metrics</h2></summary>

If you choose to deploy metrics services, grafana and prometheus will be
deployed with docker compose on manager node. You can use subcommand
`grafana-tunnel` to create a tunnel to manager node's grafana service. This will
expose the grafana service on your local machine and you will be able to inspect
the metrics.

Metrics are scraped from each worker node's `cadvisor` service.

Note that default grafana installation has username and password set to `admin`.
You will be prompted to change the password on first login.

</details>


<details>
  <summary><h2>Scaling worker instances</h2></summary>

  If you wish to scale the number of worker instances up or down, you can do so
  by re-running `d8x setup` command inside existing deployment's directory. When
  prompted to enter the number of worker instances, simply enter the new number
  of instances you wish to have.

  Setup will automatically scale the number of worker instances up or down and
  rerun any configuration steps needed. Broker and Manager servers will not be
  changed.

  **Note** that you do not need to select "setup certbot" when scaling the
  worker instances if certbot is already set up.
</details>

## SSH into machines

`d8x` cli can be used to quickly ssh into your provisioned machines.

Use the following command to do so:

```bash
d8x ssh <machine-name>
```

here `<machine-name>` is one of `manager|broker|worker-x` where `x` is a number
of a worker node.

## FAQ

<details>
  <summary>What if I need to change the configuration files?</summary>
Edit the configuration files in your local folder from which you deployed. Redeploy using the CLI, see also `d8x setup --help`.
For example `d8x setup swarm-deploy` if the config is part of "trader-backend", or `d8x setup broker-deploy` if the config is
part of "broker-server"
</details>

<details>
  <summary>Where can I find my Linode API Token?</summary>

You can generate one on the Linode website, see [the Linode guidance](https://www.linode.com/docs/products/tools/api/get-started/#get-an-access-token)

</details>

<details>
  <summary>Where can I find my Linode database cluster ID?</summary>
On the Linode website, navigate to your databases and click on the database cluster that you want to use for the backend. 
The URL uses the format `https://cloud.linode.com/databases/postgresql/YOURDBID`, for example  `https://cloud.linode.com/databases/postgresql/29109`, hence the id is 29109.
</details>

<details>
  <summary>Do I need to backup the database?</summary>

Most of the data stored in either REDIS or PostgreSQL does not have to be persistent because it is gathered from the blockchain, including payment execution data. However, referral code information is broker specific and cannot be reconstructed from on-chain data. This should typically not be a large amount of data. The relevant tables are

- referral_code
- referral_code_usage
- referral_chain

</details>

<details>
  <summary>I can't find databases on Linode, what shall I do?</summary>
Linode currently disabled provisioning of new database clusters for some customers. However,
you can use another PostgreSQL database from any provider, just select to _not_ use a Linode
database in the CLI tool and you have to allow-list the 'manager's IP address (visible in hosts.cfg).
</details>
<details>
  <summary>How can I check system health?</summary>
<p>
The CLI has a basic feature `d8x health`. However, you can also check whether you obtain data
from the different services, for example:
  
    - main REST api: for example https://api.dev.yourdomain.com/exchange-info (for your url)
    - Referral code REST api: https://referral.yourdomain.com/my-referral-codes?addr=0x015015028e98678d0e645ea4aebd25f744341a05
    - Use a websocket tool (for example websocket-test-client for Chrome) and connect to the candle API
    `wss://candles.yourdomain.com` and you should receive a response like `{"type":"connect","msg":"success","data":{}}`
    - Send a btc subscription request `{"type": "subscribe", "topic": "btc-usd:1m",}`
    and you should receive an update about every second.
    - See whether Pyth candle-stick are up: https://web-api.pyth.network/history?symbol=FX.GBP/USD&range=1H&cluster=testnet - you should get a JSON response
</p>
</details>

<details>
  <summary>One service seems to not work, how can I troubleshoot?</summary>
  
  If the broker service is not responding, login to the broker server `d8x ssh broker`,
  otherwise to the swarm-manager `d8x ssh manager`.

On the swarm manager inspect the services with `docker service ls` and look at the
log files with `docker service logs stack_api -f` (replace stack_api with the
service name you want to inspect). The logs will help you troubleshoot. Most likely
there is a misconfiguration. If so, go back to your local deployment directory,
edit the configs, and redeploy with `d8x setup swarm-deploy`.

On the broker server, inspect the services with `docker ps` and look at the log
files with `cd broker` and `docker compose logs -f`. Redeploy changed configs
via `d8x setup broker-deploy`

Sometimes swarm cluster deployment ingress network can get stuck on manager
server. This might result in HTTP 503 errors or `Connection refused` message
when trying to curl swarm services on manager server. To fix this you can re-run
`d8x setup swarm-deploy` command which attempts to fix "broker" ingress network
after deploying swarm services. There is also individual subcommand `d8x
fix-ingress` which does only the ingress network fixing part, but requires to
rerun `swarm-deploy` command afterwards.

</details>
<details>
  <summary>How do I update the swarm server software images to a new version?</summary>

**Via CLI**

You can update services running in docker swarm and broker server by using the
`d8x update` command. Command `update` will walk you through the update process.
You will be prompted to select which services you want to update in the swarm
cluster and then in the broker server.

For swarm services, you will need to  provide a fully qualified url of docker
image for the service to update to.

Example:

Let's say our main `api` service is running as
`ghcr.io/d8-x/d8x-trader-main:main` image in our swarm setup. Now, if you want
to update service `api` to the latest version of
`ghcr.io/d8-x/d8x-trader-main:main` (`main` tag), simply updating to use
`ghcr.io/d8-x/d8x-trader-main:main` image will not work. This is because swarm
nodes will see that this image with main tag is already downloaded and even
though there is a newer version of `main` image in container registry, it will
not pull it (because the tag is the same). In order to force the upate of a
specific tag that is already available and running in swarm, you have to specify
the sha hash of the image. For example
`ghcr.io/d8-x/d8x-trader-main:main@sha256:2ce51e825a559029f47e73a73531d8a0b10191c6bc16950649036edf20ea8c35`

For broker server services, update will attempt to update services to the latest
version available. This is because broker services are running in docker compose
and the `update` command simply removes old containers and images and pulls new
ones.



**Manually**

You login to the server where your software resides (e.g., the broker-server, or the
swarm-manager for which you can get the ip with `d8x ip manager`).

- Find the hash (sha256:...) of the service you want to update by navigating to the root of the github repository, click on packages (or [here](https://github.com/orgs/D8-X/packages)), choose the package and version you want to update and the hash is displayed on the top. For example, choose "trader-main" and click the relevant version for the main broker services.
- Find the name of the service via `docker service ls`
- Now you can update the backend-service application by using the docker update command. For example:

```
docker service update --image "ghcr.io/d8-x/d8x-trader-main:dev@sha256:aea8e56d6077c733a1d553b4291149712c022b8bd72571d2a852a5478e1ec559" stack_api
```

See the [Update Runbook](./UPDATE_RUNBOOK.md) for more guidelines and more
details how updating services works.

</details>

<details>
  <summary>Supported platforms</summary>

D8X-CLI is tested and runs natively on Linux. The CLI also runs on MacOS. For MacOS you
need to build the application on your own (pay attention at the Go version), and you
need to manually install ansible and terraform.

D8X-CLI is not tested on Windows and will probably not work, we would
recommend using WSL2 to run D8X-CLI on Windows.

</details>


# Metrics

Metrics is advanced feature and is not required to run the trader backend.
However, we do recommend to deploy metrics stack in order to be able to monitor
resource usage of deployed services.

Metrics stack will be deployed automatically when running `d8x setup` command.
Or you can manually deploy metrics via `d8x setup metrics-deploy` command.

## Metrics services ports

- Grafana is published at `127.0.0.1` on port `4002` on manager node
- Cadvisor instances are deployed on port `4003` on each worker node.

By default Prometheus instance is not published and is only accessible from
grafana instance. Grafana and cadvisor ports are not accessible to the public
network.


# Database backups

You can use cli subcommand `backup-db` to backup the database that you provided
during the setup process. The `backup-db` will take the database dsn from your
`d8x.conf.json` file and connect via ssh to the manager server to create a
backup. Backup SQL dump will be downloaded in your current working directory, or
to directory specified by `--output-dir` flag. 

Backup files will be named in the following format:
`backup-<server-label-prefix>-<date-time>.dump.sql` for example
`backup-d8x-cluster-test-1234-2024-01-17-19-23-50.dump.sql`

Example
```bash
$ d8x backup-db

Backing up database...

Determining postgres version
Postgres server at pg-30826a8f-quantena-6be1.a.aivencloud.com version: 13.13
Ensuring pg_dump is installed on manager server (postgresql-client-16)
Creating database defaultdb backup
Backup file size: 0.389526 MB
Database defaultdb backup file was downloaded and copied to /home/d8x/deployment/backup-d8x-cluster-test-1234-2024-01-17-20-16-09.dump.sql
Removing backup file from server
```

## Using `backup-db` as cronjob

You can add a crontab entry on your local machine to periodicaly backup your
database. When adding crontab entry we recommend to use `--chdir` global flag to
specify absolute path to your deployment directory. Otherwise, `backup-db` might
fail to find the deployment directory. If you also specify the `--output-dir`
with `--chdir` flag, the output directory will be relative (unless absolute path
is specified) to the directory specified by `--chdir`.

For example, if your deployment is in /my/deployment/dir and you want to store
the backups in /my/deployment/dir/db-backups you would run a command like this:

```bash
d8x --chdir /my/deployment/dir backup-db --output-dir db-backups
```

To run the command above as a cronjob every day at 16:00 you would add the
following entry to your crontab:
```bash
0 16 * * * d8x --chdir /my/deployment/dir backup-db --output-dir db-backups
```

Note that the machine where you add the crontab entry must be powered on in
order for the cronjob to run.

## Restoring the backups
Backups are plain sql scripts created with `pg_dump`. You can use `psql` or any
other postgres client to load the database backup into a database. **Note** that
you should always use empty database when restoring a backup. If you attempt to
load the database backup into an existing database, your data might get
corrupted.

To restore a backup:
```bash
psql -U user -h host -p port -d databasename < /path/to/your-backup.dump.sql
```

# How to connect to AWS RDS database on your local machine

In order to access AWS RDS instance locally, you need to create a SSH tunnel
using manager server as jump host.

1. cd to your deployment directory
2. Get the manager public ip address by inspecting `hosts.cfg` or running `d8x ip manager`
3. Open `aws_rds_postgres.txt` to find the `host` and `port` of your RDS postgres instance
4. Make a tunnel `ssh -L <LOCAL_PORT>:<AWS_RDS_HOST>:<AWS_RDS_PORT> -N d8xtrader@<MANAGER_PUBLIC_IP> -i ./id_ed25519`
	- <LOCAL_PORT> - the port on which tunneled postgres instance will be exposed on your localhost
	- <AWS_RDS_HOST> - host value from `aws_rds_postgres.txt`
	- <AWS_RDS_PORT> - port value from `aws_rds_postgres.txt`
	- <MANAGER_PUBLIC_IP> - your manager server's public ip address
5. Use any postgres client (dbeaver) and login to postgres instance with localhost and <LOCAL_PORT>
	- username: `user` value from `aws_rds_postgres.txt`
	- password: `password` value from `aws_rds_postgres.txt`
6. Inspect the database

# Broker .env

You can place `.env` file in the `broker-server` directory. This file will be
copied to the broker server and used as env file for broker compose services.