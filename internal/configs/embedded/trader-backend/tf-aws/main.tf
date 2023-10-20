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
  key_name   = "d8x-cluster-ssh-key"
  public_key = var.authorized_key
}

// Find latest ubuntu 22.04 LTS 
data "aws_ami" "ubuntu" {
  most_recent = true
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-20230608"]
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
    Name = "d8x-cluster-vpc"
  }
}


resource "aws_eip" "manager_ip" {
  tags = {
    Name = "d8x-cluster-eip"
  }
}

resource "aws_nat_gateway" "public_nat" {
  allocation_id = aws_eip.manager_ip.id
  subnet_id     = aws_subnet.public_subnet.id


  tags = {
    Name = "d8x-cluster-nat-gateway"
  }

  # To ensure proper ordering, it is recommended to add an explicit dependency
  # on the Internet Gateway for the VPC.
  depends_on = [aws_internet_gateway.d8x_igw, aws_subnet.public_subnet]
}

resource "aws_subnet" "public_subnet" {
  vpc_id     = aws_vpc.d8x_cluster_vpc.id
  cidr_block = local.subnets[0]

  tags = {
    Name = "d8x-cluster-subnet_public"
  }
  map_public_ip_on_launch = true
}

resource "aws_subnet" "workers_subnet" {
  vpc_id     = aws_vpc.d8x_cluster_vpc.id
  cidr_block = local.subnets[1]
  tags = {
    Name = "d8x-cluster-subnet_workers"
  }
  availability_zone = "${var.region}a"
}

// Aditional private subnet in different az for rds
resource "aws_subnet" "private_subnet_2" {
  vpc_id     = aws_vpc.d8x_cluster_vpc.id
  cidr_block = local.subnets[2]
  tags = {
    Name = "d8x-cluster-subnet_private_2"
  }
  availability_zone = "${var.region}b"
}

// attach igw to vpc
resource "aws_internet_gateway" "d8x_igw" {
  vpc_id = aws_vpc.d8x_cluster_vpc.id
  tags = {
    Name = "d8x-cluster-igw"
  }
}

# Provision postgres on RDS
resource "aws_db_subnet_group" "pg_subnet" {
  name       = "d8x-cluster-postgres-subnet"
  subnet_ids = [aws_subnet.workers_subnet.id, aws_subnet.private_subnet_2.id]
  tags = {
    Name = "private subnet association"
  }
}

resource "random_password" "db_password" {
  length  = 28
  special = false
}

resource "aws_db_instance" "pg" {
  identifier             = "d8x-cluster-pg"
  instance_class         = var.db_instance_class
  allocated_storage      = 5
  engine                 = "postgres"
  engine_version         = "15.2"
  username               = "d8xtrader"
  password               = random_password.db_password.result
  vpc_security_group_ids = [aws_security_group.db_access.id]
  db_subnet_group_name   = aws_db_subnet_group.pg_subnet.name
  # vpc_security_group_ids = [aws_security_group.rds.id]
  # parameter_group_name = aws_db_parameter_group.education.name
  publicly_accessible   = false
  skip_final_snapshot   = true
  max_allocated_storage = 50
}

variable "pg_details" {
  default = <<EOF
host: %s
user: %s
port: %s
password: %s
  EOF
}

resource "local_file" "rds_db_password" {
  depends_on = [aws_db_instance.pg]
  content    = format(var.pg_details, aws_db_instance.pg.address, aws_db_instance.pg.username, aws_db_instance.pg.port, random_password.db_password.result)
  filename   = var.rds_creds_filepath
}

# resource "aws_eip" "manager_public_ip" {
#   tags = {
#     Name = "d8x-cluster-manager-public-ip"
#   }
# }

# resource "aws_eip_association" "eip_assoc" {
#   instance_id   = aws_instance.manager.primary_network_interface_id
#   allocation_id = aws_eip.manager_public_ip.id
# }

resource "aws_instance" "manager" {
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.worker_size
  key_name      = aws_key_pair.d8x_cluster_ssh_key.key_name

  subnet_id                   = aws_subnet.public_subnet.id
  associate_public_ip_address = true
  security_groups             = [aws_security_group.ssh_docker_sg.id, aws_security_group.http_access.id, aws_security_group.nfs_access.id]

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "manager")
  }
}

resource "aws_instance" "broker_server" {
  count         = var.create_broker_server ? 1 : 0
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.worker_size
  key_name      = aws_key_pair.d8x_cluster_ssh_key.key_name

  subnet_id                   = aws_subnet.public_subnet.id
  associate_public_ip_address = true
  security_groups             = [aws_security_group.ssh_docker_sg.id, aws_security_group.http_access.id]

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "broker-server")
  }
}

resource "aws_instance" "nodes" {
  count = var.num_workers

  ami           = data.aws_ami.ubuntu.id
  instance_type = var.worker_size
  key_name      = aws_key_pair.d8x_cluster_ssh_key.key_name
  subnet_id     = aws_subnet.workers_subnet.id

  security_groups = [aws_security_group.ssh_docker_sg.id]

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "worker-${count.index + 1}")
  }
}


variable "ssh_jump_host_cfg_path" {
  default = "./manager_ssh_jump.conf"
}

# Geneate ansible inventory with jump host for workers
resource "local_file" "hosts_cfg" {
  depends_on = [aws_instance.nodes, aws_instance.manager]
  content    = <<EOF
[managers]
${aws_instance.manager.public_ip} manager_private_ip=${aws_instance.manager.private_ip} hostname=manager-1

[workers]
%{for index, ip in aws_instance.nodes[*].private_ip~}
${ip} worker_private_ip=${ip} hostname=${format("worker-%02d", index + 1)} 
%{endfor~}

[workers:vars]
ansible_ssh_common_args="-J jump_host -F ${var.ssh_jump_host_cfg_path}"

%{if var.create_broker_server}
[broker]
${aws_instance.broker_server[0].public_ip} private_ip=${aws_instance.broker_server[0].private_ip}
%{endif~}
EOF

  filename = "hosts.cfg"
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
  content  = format(var.ssh_jump_host, aws_instance.manager.public_ip)
  filename = var.ssh_jump_host_cfg_path
}

