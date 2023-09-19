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
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_vpc" "d8x_cluster_vpc" {
  cidr_block       = "10.0.0.0/16"
  instance_tenancy = "default"

  tags = {
    Name = "d8x-cluster-vpc"
  }
}

resource "aws_security_group" "allow_ssh" {
  name   = "allow-all-sg"
  vpc_id = aws_vpc.d8x_cluster_vpc.id
  ingress {
    cidr_blocks = [
      "0.0.0.0/0"
    ]
    from_port = 22
    to_port   = 22
    protocol  = "tcp"
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_subnet" "workers_subnet" {
  vpc_id     = aws_vpc.d8x_cluster_vpc.id
  cidr_block = "10.0.1.0/24"
  tags = {
    Name = "d8x-cluster-subnet_workers"
  }
}

resource "aws_subnet" "public_subnet" {
  vpc_id     = aws_vpc.d8x_cluster_vpc.id
  cidr_block = "10.0.2.0/24"

  tags = {
    Name = "d8x-cluster-subnet_public"
  }
  map_public_ip_on_launch = true
}

resource "aws_instance" "manager" {
  ami           = data.aws_ami.ubuntu.id
  instance_type = var.worker_size
  key_name      = aws_key_pair.d8x_cluster_ssh_key.key_name

  subnet_id = aws_subnet.public_subnet.id

  security_groups = [aws_security_group.allow_ssh.id]

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "manager")
  }
}

resource "aws_instance" "nodes" {
  count = var.num_workers

  ami           = data.aws_ami.ubuntu.id
  instance_type = var.worker_size
  key_name      = aws_key_pair.d8x_cluster_ssh_key.key_name

  subnet_id = aws_subnet.workers_subnet.id

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "worker-${count.index + 1}")
  }
}


# Geneate ansible inventory
# resource "local_file" "hosts_cfg" {
#   depends_on = [linode_instance.manager, linode_instance.nodes, aws_instance.nodes.*.]
#   content = templatefile("inventory.tpl",
#     {
#       manager_public_ip   = linode_instance.manager.ip_address
#       manager_private_ip  = linode_instance.manager.private_ip_address
#       workers_public_ips  = linode_instance.nodes.*.ip_address
#       workers_private_ips = linode_instance.nodes.*.private_ip_address
#       broker_public_ip    = var.create_broker_server ? linode_instance.broker_server[0].ip_address : ""
#       broker_private_ip   = var.create_broker_server ? linode_instance.broker_server[0].private_ip_address : ""
#     }
#   )
#   filename = "hosts.cfg"
# }

