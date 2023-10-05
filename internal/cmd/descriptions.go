package cmd

// MainDescription is the description text for d8x cli tool
const MainDescription = `D8X Perpetual Exchange broker backend setup and management CLI tool 

Running d8x without any subcommands or init command will perform initalization
of ./.d8x-config directory (--config-directory), as well as prompt you to
install any missing dependencies such as ansible or terraform.

D8X CLI relies on the following external tools: terraform, ansible. You can
manually install them or let the cli attempt to perform the installation of
these dependencies automatically. Note that for automatic installation you will
need to have python3 and pip installed on your system

For cluster provisioning and configuration, see the setup command and its 
subcommands. Run d8x setup --help for more information.
`

const SetupDescription = `Command setup performs complete D8X cluster setup.

Setup should be performed only once! Once cluster is provisioned and deployed,
you should use one of the individual setup subcommands to perform any individual
operations such as swarm or broker deployments. Calling setup on provisioned
cluster might result in data corruption: password.txt overwrites, ssh key
overwrites, misconfiguration/destruction of servers, etc.

In essence setup calls the following subcommands in sequence:
	- provision
	- configure
	- broker-deploy
	- broker-nginx
	- swarm-deploy
	- swarm-nginx

Command provision performs resource provisioning with terraform.

Command configure performs configuration of provisioned resources with ansible.

Command broker-deploy performs broker-server deployment.

Command broker-nginx performs nginx + certbot setup for broker-server
deployment.

Command swarm-deploy performs d8x-trader-backend docker swarm cluster
deployment.

Command swarm-nginx performs nginx + certbot setup for d8x-trader-backend docker
swarm deployment on manager server.

See individual command's help for information and more details how each step operates.

Files created by setup and it's subcommands:
	- hosts.cfg - ansible inventory file
	- id_ed25519 - ssh key used to access servers (added to each provisioned server)
	- id_ed25519.pub - public key of id_ed25519 
	- password.txt - default user password on all servers
	- pg.crt - postgress database root CA certificate (downloaded from server provider)
	- aws_rds_postgres.txt - aws postgres instance credentials (only for AWS provider)
	- manager_ssh_jump.conf - ssh config file for manager server to be used as jump host (only for AWS provider)
`

const ProvisionDescription = `Command provision performs resource provisioning with terraform.

Currently supported providers are:
	- linode
	- aws	

Provisioning Linode resources requires you to provide linode token, database id and region
information. Database provisioning is not included by default, since it takes up
to half an hour to complete. Therefore, you will need to provision database manually, before 
running the provision command.

Provisioning AWS resources will require you to provide your AWS access and
secret keys. We recommend creating a dedicated IAM user with sufficient
permissions to manage your VPCs, EC2 instances, RDS instances. When using AWS 
provider, RDS Postgres instance is provisioned automatically.
`

const SwarmDeployDescription = `Command swarm-deploy performs docker swarm cluster deployment

This command establishes ssh access to your manager node, copies required
configurations files to the server and deploys the docker stack which consists 
of d8x trader backend services (main, history, referral, candles).

Subsequent runs will reupload modified configuration files and redeploy the
stack. Old running stack will be removed, therefore you should make sure to
backup any data if needed.
`

const SwarmNginxDescription = `Command swarm-nginx performs nginx + certbot setup for d8x trader backend docker swarm deployment on manager node.`

const ConfigureDescription = `Command configure performs configuration of provisioned resources with ansible.`
