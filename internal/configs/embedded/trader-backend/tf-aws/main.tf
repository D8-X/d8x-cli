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

resource "aws_instance" "nodes" {
  count = var.num_workers

  ami           = var.ami_image_id
  instance_type = var.worker_size
  key_name      = aws_key_pair.d8x_cluster_ssh_key.key_name

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

