resource "aws_eip" "manager_ip" {
  tags = {
    Name = "${var.server_label_prefix}-eip"
  }
}

// NAT gateway for worker nodes
resource "aws_nat_gateway" "public_nat" {
  allocation_id = aws_eip.manager_ip.id
  subnet_id     = var.public_subnet_id

  tags = {
    Name = "${var.server_label_prefix}-nat-gateway"
  }
}

resource "aws_instance" "manager" {
  ami           = var.ami_image_id
  instance_type = var.manager_instance_type
  key_name      = var.keypair_name

  subnet_id                   = var.public_subnet_id
  associate_public_ip_address = true
  vpc_security_group_ids      = var.security_group_ids_manager

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "manager")
  }

  # Set 30 GB for worker nodes
  root_block_device {
    volume_size = 30
  }
}

# Worker nodes
resource "aws_instance" "nodes" {
  count = var.num_workers

  ami           = var.ami_image_id
  instance_type = var.worker_instance_type
  key_name      = var.keypair_name
  subnet_id     = var.workers_subnet_id

  # Always use vpc_security_group_ids instead of security_group to prevent
  # rebuilds. See
  # https://github.com/hashicorp/terraform/issues/7221#issuecomment-227156871
  vpc_security_group_ids = var.security_group_ids_workers

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "worker-${count.index + 1}")
  }

  # Set 25 GB for worker nodes
  root_block_device {
    volume_size = 25
  }
}



