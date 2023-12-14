

resource "aws_eip" "manager_ip" {
  tags = {
    Name = "${var.server_label_prefix}-eip"
  }
}

resource "aws_nat_gateway" "public_nat" {
  allocation_id = aws_eip.manager_ip.id
  subnet_id     = aws_subnet.public_subnet.id

  tags = {
    Name = "${var.server_label_prefix}-nat-gateway"
  }

  # To ensure proper ordering, it is recommended to add an explicit dependency
  # on the Internet Gateway for the VPC.
  depends_on = [aws_internet_gateway.d8x_igw, aws_subnet.public_subnet]
}

resource "aws_internet_gateway" "d8x_igw" {
  vpc_id = var.vpc_id
  tags = {
    Name = "${var.server_label_prefix}-igw"
  }
}

resource "aws_instance" "manager" {
  ami           = var.ami_image_id
  instance_type = var.manager_instance_type
  key_name      = var.keypair_name

  subnet_id                   = var.public_subnet_id
  associate_public_ip_address = true
  security_groups             = [aws_security_group.ssh_docker_sg.id, aws_security_group.http_access.id, aws_security_group.nfs_access.id]

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

  security_groups = [aws_security_group.ssh_docker_sg.id, aws_security_group.cadvisor_port.id]

  tags = {
    Name = format("%s-%s", var.server_label_prefix, "worker-${count.index + 1}")
  }

  # Set 25 GB for worker nodes
  root_block_device {
    volume_size = 25
  }
}



