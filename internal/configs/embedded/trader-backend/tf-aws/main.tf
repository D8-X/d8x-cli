terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region     = var.region
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
}

// Create d8x cluster keypair
resource "aws_key_pair" "d8x_cluster_ssh_key" {
  // https://github.com/hashicorp/terraform-provider-aws/issues/10497
  key_name   = format("%s-%s-%s", var.server_label_prefix, "cluster-ssh-key", md5(var.authorized_key))
  public_key = var.authorized_key
}

// Find latest ubuntu 22.04 LTS 
data "aws_ami" "ubuntu" {
  most_recent = true
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20240904"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  filter {
    name   = "state"
    values = ["available"]
  }
}

resource "aws_vpc" "d8x_cluster_vpc" {
  cidr_block           = "10.0.0.0/16"
  instance_tenancy     = "default"
  enable_dns_hostnames = true

  tags = {
    Name = "${var.server_label_prefix}-vpc"
  }
}

resource "aws_subnet" "public_subnet" {
  vpc_id     = aws_vpc.d8x_cluster_vpc.id
  cidr_block = local.subnets[0]

  tags = {
    Name = "${var.server_label_prefix}-subnet_public"
  }
  map_public_ip_on_launch = true
}

resource "aws_subnet" "workers_subnet" {
  vpc_id     = aws_vpc.d8x_cluster_vpc.id
  cidr_block = local.subnets[1]
  tags = {
    Name = "${var.server_label_prefix}-subnet_workers"
  }
  availability_zone = "${var.region}a"
}


// Create internet gateway for the vpc
resource "aws_internet_gateway" "d8x_igw" {
  vpc_id = aws_vpc.d8x_cluster_vpc.id
  tags = {
    Name = "${var.server_label_prefix}-igw"
  }
}

# Create swarm manager and worker nodes + rds
module "swarm_servers" {
  source = "./swarm"
  count  = var.create_swarm ? 1 : 0

  server_label_prefix   = var.server_label_prefix
  vpc_id                = aws_vpc.d8x_cluster_vpc.id
  worker_instance_type  = var.worker_size
  manager_instance_type = var.worker_size
  num_workers           = var.num_workers
  ami_image_id          = data.aws_ami.ubuntu.id
  keypair_name          = aws_key_pair.d8x_cluster_ssh_key.key_name
  public_subnet_id      = aws_subnet.public_subnet.id
  workers_subnet_id     = aws_subnet.workers_subnet.id
  region                = var.region
  // Manager must have ssh (public);docker swarm (internal);http (public);nfs (internal) ports open 
  security_group_ids_manager = [aws_security_group.ssh_docker_sg.id, aws_security_group.http_access.id, aws_security_group.nfs_access.id]
  security_group_ids_workers = [aws_security_group.ssh_docker_sg.id, aws_security_group.cadvisor_port.id]
  subnets                    = local.subnets

  // PG RDS vars
  db_instance_class  = var.db_instance_class
  rds_creds_filepath = var.rds_creds_filepath

  depends_on = [aws_internet_gateway.d8x_igw, aws_subnet.public_subnet, aws_subnet.workers_subnet]
}

resource "aws_instance" "broker_server" {
  count = var.create_broker_server ? 1 : 0

  ami           = data.aws_ami.ubuntu.id
  instance_type = var.worker_size
  key_name      = aws_key_pair.d8x_cluster_ssh_key.key_name

  subnet_id                   = aws_subnet.public_subnet.id
  associate_public_ip_address = true
  vpc_security_group_ids      = [aws_security_group.ssh_docker_sg.id, aws_security_group.http_access.id]

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "broker-server")
  }
}

# Geneate ansible inventory with jump host for workers
resource "local_file" "hosts_cfg" {
  content = <<EOF
%{if var.create_swarm}

[managers]
${module.swarm_servers[0].manager.public_ip} manager_private_ip=${module.swarm_servers[0].manager.private_ip} hostname=manager-1

[workers]
%{for index, ip in module.swarm_servers[0].workers[*].private_ip~}
${ip} worker_private_ip=${ip} hostname=${format("worker-%02d", index + 1)} 
%{endfor~}

[workers:vars]
ansible_ssh_common_args="-J jump_host -F ${var.ssh_jump_host_cfg_filename}"

%{endif~}

%{if var.create_broker_server}
[broker]
${aws_instance.broker_server[0].public_ip} private_ip=${aws_instance.broker_server[0].private_ip}
%{endif~}
EOF

  filename = var.host_cfg_path
}

# Template for manager as jump host ssh config. We can later use ProxyJump
# jump_host for accessing workers via ansible.
variable "ssh_jump_host" {
  default = <<EOF
  Host jump_host
    Hostname %s
    User ubuntu
    IdentityFile ./id_ed25519
    Port 22
  EOF
}

# Generate a small ssh config workaround for accessing workers through manager
# as a ProxyJump
resource "local_file" "jump_host_ssh_config" {
  count    = var.create_swarm ? 1 : 0
  content  = format(var.ssh_jump_host, module.swarm_servers[0].manager.public_ip)
  filename = "../${var.ssh_jump_host_cfg_filename}"
}

