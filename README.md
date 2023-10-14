# D8X CLI

D8X CLI is a tool which helps you to easiliy spin up d8x-trader-backend and
other d8x broker services.

Setup includes provisioning resources on supported cloud providers, configuring servers, deploying swarm cluster and individual services.

For more information check and usage out the `d8x help` command.

## Building From Source

```bash
go build -o d8x ./main.go
sudo mv d8x /usr/bin/d8x
```

## Using A Release

Head to [releases](https://github.com/D8-X/d8x-cli/releases), download and
extract the d8x binary and place it in your `PATH`. 

Note that binary releases are provided only for Linux. To run D8X-CLI on other
platforms you will need to [build it from source](#building-from-source). See
FAQ supported platforms for details.

## Usage
### Before You Start The CLI
* The CLI is built for Linux. The CLI allows to deploy on Linode and AWS. Linode is thoroughly tested, AWS less so. You can use an external database. 
* With Linode, or when using an externally managed Postgres database, setup the database cluster and create a database (any name is fine it's called 'history' in our pre-defined config) 
* Have a broker key ready, and a broker executor key. The address belonging to the executor will need to be entered as 'allowed executors' in the setup for broker server (more details will follow, this is a heads-up).
	* Fund the executor wallet with gas tokens (ETH on zkEVM) and monitor the wallet for its ETH balance 
* Have multiple private RPCs for Websocket and HTTP ready. As of writing of this document, only Quicknode provides Websocket RPCs for Polygon's zkEVM
* Consider running your own Hermes price service to reliably stream prices: [details](https://docs.pyth.network/documentation/pythnet-price-feeds/hermes). The service endpoint will have to be added to the configuration file (variable priceServiceWSEndpoint of the candles-service -- more details on configs will follow, this is a heads-up)

### Setup

Performing a complete servers provisioning + configuration and services
deployment can be done with the `setup` command:

```bash
d8x setup
```

The `setup` command is an aggregate of multiple individual subcommands such as
`provision`, `configure`, `swarm-deploy`, etc. Setup will walk you through
entire process of provisioning servers and deploying all the services. See `d8x
setup --help` for more information and available `setup` subcommands.

#### Provisioning and Configuration
Setup will start with provisioning servers with terraform and configuring them
with ansible. Depending on your selected server provider, you will need to
provide API tokens, access keys and other necessary information.

After provisioning and configuration is done a couple of files will be created
in your current working directory:

  - hosts.cfg - ansible inventory file
  - id_ed25519 - ssh key used to access servers (added to each provisioned server)
  - id_ed25519.pub - public key of id_ed25519 
  - password.txt - default user password on all servers
  - pg.crt - postgress database root CA certificate (downloaded from server provider)
  - aws_rds_postgres.txt - aws postgres instance credentials (only for AWS provider)
	- manager_ssh_jump.conf - ssh config file for manager server to be used as jump host (only for AWS provider)

Do not delete these files, as they are to establish connections to your servers
when running individual `setup` subcommands.

For Linode provider - you need to make sure to provision database instance by
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
as `.env` file. Particularly important configuration file that you should
supply with valid values is `live.rpc.json`. As the default values do not contain
websockets rpc endpoints which are necessary to run the services.

Nginx + certbot setup is completely analogous to broker-server setup, but
involves more domains for services.


To run only trader backend swarm deployment:
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

## Configuration Files
Configuration files are key and the most involved part to setup D8X Perpetuals Backend:
find out how to configure the system in the
[README](README_CONFIG.md).

## FAQ

<details>
  <summary>What if I need to change the configuration files?</summary>
Edit the configuration files in your local folder from which you deployed. Redeploy using the CLI, see also `d8x setup --help`.
For example `d8x setup swarm-deploy` if the config is part of "trader-backend", or `d8x setup broker-deploy` if the config is
part of "broker-server"
</details>



<details>
  <summary>How do I update the server software images to a new version?</summary>

  You login to the server where your software resides (e.g., the broker-server, or the
  swarm-manager for which you can get the ip with `d8x ip manager`).

  -  Find the hash (sha256:...) of the service you want to update by navigating to the root of the github repository, click on packages (or [here](https://github.com/orgs/D8-X/packages)), choose the package and version you want to update and the hash is displayed on the top. For example, choose "trader-main" and click the relevant version for the main broker services.
  -  Find the name of the service via `docker service ls`
  -  Now you can update the backend-service application by using the docker update command. For example:
  
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


