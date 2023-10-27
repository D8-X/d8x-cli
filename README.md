# D8X CLI

D8X CLI is a tool which helps you to easiliy spin up d8x-trader-backend and
other d8x broker services.

Setup includes provisioning resources on supported cloud providers, configuring
servers, deploying swarm cluster and individual services.

## Building From Source

```bash
go build -o d8x ./main.go
sudo mv d8x /usr/bin/d8x
```

Check out the `d8x help` command.

## Using A Release

Head to [releases](https://github.com/D8-X/d8x-cli/releases), download and
extract the d8x binary and place it in your `PATH`.

Note that binary releases are provided only for Linux. To run D8X-CLI on other
platforms you will need to [build it from source](#building-from-source). See
FAQ supported platforms for details.

## Before You Start The CLI

- The CLI is built for Linux. The CLI allows to deploy on Linode and AWS. Linode is thoroughly tested, AWS less so.
- The CLI gives you the choice of using a cloud-provider database or an external database.
- With Linode, or when using an externally managed Postgres database, setup the database cluster and create a database. Any name for the db is fine. The db is called 'history' in our pre-defined config.
  - Have the database id ready, which you can read from the URL after browsing to the database on the Linode website, for example `https://cloud.linode.com/databases/postgresql/29109` the number 20109 is the id that the CLI tool asks for
- Have a broker key/address and a broker executor key/address ready. The broker address is the address of the Whitelabelling partner that is paid trader fees (that are then redistributed according to the [referral system](https://github.com/D8-X/referral-system)). The executor executes referral
payments. The address belonging to the executor will need to be entered as 'allowed executors' in the setup for broker server (more details will follow, this is a heads-up).
  - Fund the executor wallet with gas tokens (ETH on zkEVM) and monitor the wallet for its ETH balance
- Decide on the broker fee. The broker fee is paid from the trader to the broker and potentially to referrers. The broker fee is relative to the notional position size. The broker fee is entered in tenth of a basis point ("tbps"),
  that is, the percentage multiplied by 1000, so that `0.06% = 6 bps = 60 tbps` or `0.10% = 10 bps = 100 tbps`
- Have multiple private RPCs for Websocket and HTTP ready. As of writing of this document, only Quicknode provides Websocket RPCs for Polygon's zkEVM
- You need to be able to access your Domain Name Service provider so you can create DNS records
  Typically a config entry looks something like this:

  |        Hostname        | Type |  TTL   |      Data       |
  | :--------------------: | :--: | :----: | :-------------: |
  | api.dev.yourdomain.com |  A   | 1 hour | 139.144.112.122 |

  <details>
    <summary>Recommended Domain Name Entries</summary>
    There are different REST APIs and WebSockets, which we map to the public IPs of the servers. 
    The services and example names for the backend are as follows:
    
  	* api.dev.yourdomain.com: main REST API points to “Swarm manager”
  	* ws.dev.yourdomain.com: main WebSocket points to “Swarm manager”
  	* history.dev.yourdomain.com: historical data REST API points to “Swarm manager”
  	* referral.dev.yourdomain.com: referral code REST API points to “Swarm manager”
    	* candles.dev.yourdomain.com: The Candle Server websocket points to “Swarm manager”
    	* broker.dev.yourdomain.com: The broker server has its own IP

  Both IP addresses, for the manager and broker server, will be shown to you during the setup.
  </details>

- Consider running your own Hermes price service to reliably stream prices: [details](https://docs.pyth.network/documentation/pythnet-price-feeds/hermes). The service endpoint will have to be added to the configuration file (variable priceServiceWSEndpoints of the candles-service -- more details on configs will follow, this is a heads-up)

## Configuration Files

Configuration files are key and the most involved part to setup D8X Perpetuals Backend:
find out how to configure the system in the
[README](README_CONFIG.md).

## Usage Of The D8X CLI Tool

### Setup

Performing a complete servers provisioning + configuration and services
deployment can be done with the `setup` command:

```bash
d8x setup
```

The `setup` command is an aggregate of multiple individual subcommands such as
`provision`, `configure`, `swarm-deploy`, etc. Setup command will walk you
through entire process of provisioning servers and deploying all the services.
See `d8x setup --help` for more information and available `setup` subcommands.

#### Provisioning and Configuration

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
policies to allow access from all of the provisioned servers ip addresses.

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

#### Broker Server

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

#### Trader backend (Swarm)

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

### Teardown

If you wish to completely remove any provisioned resources, you can do so by
running `tf-destroy` command.

```bash
d8x tf-destroy
```

**Note that this action is irreversible**

### SSH into machines

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

</details>
<details>
  <summary>How do I update the swarm server software images to a new version?</summary>

You login to the server where your software resides (e.g., the broker-server, or the
swarm-manager for which you can get the ip with `d8x ip manager`).

- Find the hash (sha256:...) of the service you want to update by navigating to the root of the github repository, click on packages (or [here](https://github.com/orgs/D8-X/packages)), choose the package and version you want to update and the hash is displayed on the top. For example, choose "trader-main" and click the relevant version for the main broker services.
- Find the name of the service via `docker service ls`
- Now you can update the backend-service application by using the docker update command. For example:

```
docker service update --image "ghcr.io/d8-x/d8x-trader-main:dev@sha256:aea8e56d6077c733a1d553b4291149712c022b8bd72571d2a852a5478e1ec559" stack_api
```

</details>

<details>
  <summary>Supported platforms</summary>

D8X-CLI is tested and runs natively on Linux. MacOS might work, but you will
need to manually install ansible and terraform on your system.

D8X-CLI is not tested on Windows and will most probably not work, we would
recommend using WSL2 to run D8X-CLI on Windows.

</details>
