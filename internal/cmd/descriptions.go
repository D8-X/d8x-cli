package cmd

// MainDescription is the description text for d8x cli tool
const MainDescription = `CLI for provisioning, deploying, and managing D8X trader backend infrastructure.

Run d8x setup --help for available setup subcommands.
`

const SetupDescription = `Command setup performs D8X cluster setup.

Subcommands (in order for a new deployment):
	1. new-env        - create environment in infra repo
	2. provision      - provision servers with terraform
	3. configure      - configure servers with ansible
	4. swarm-deploy   - deploy trader backend swarm
	5. swarm-nginx    - deploy nginx + SSL for swarm
	6. broker-deploy  - deploy broker server
	7. broker-nginx   - deploy nginx + SSL for broker
	8. staging-origins - manage nginx origin whitelist

Infrastructure configs are stored in the backend-nginx-infra-config GitHub repo.
Secrets are loaded from Bitwarden or .env file.
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
of d8x trader backend services (main, history, candles).

Subsequent runs will reupload modified configuration files and redeploy the
stack. Old running stack will be removed, therefore you should make sure to
backup any data if needed.
`

const SwarmNginxDescription = `Command swarm-nginx performs nginx + certbot setup for d8x trader backend docker swarm deployment on manager node.`

const ConfigureDescription = `Command configure performs configuration of provisioned resources with ansible.`

const DeployMetricsDescription = `Command metrics-deploy configures and deploys prometheus and grafana instances on manager node. `
