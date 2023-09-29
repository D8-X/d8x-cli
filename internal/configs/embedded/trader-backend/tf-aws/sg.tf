// Allow ssh access to anyone, and docker-swarm ports between cluster nodes only
resource "aws_security_group" "ssh_docker_sg" {
  name_prefix = "d8x-cluster-sg"
  vpc_id      = aws_vpc.d8x_cluster_vpc.id

  tags = {
    Name = "d8x-cluster-allow-ssh-docker-swarm"
  }

  // Allow ssh port
  ingress {
    cidr_blocks = [
      "0.0.0.0/0"
    ]
    from_port = 22
    to_port   = 22
    protocol  = "tcp"
  }
  // Allow docker swarm ports between cluster nodes
  ingress {
    cidr_blocks = local.subnets
    from_port   = 2377
    to_port     = 2377
    protocol    = "tcp"
  }
  ingress {
    cidr_blocks = local.subnets
    from_port   = 7946
    to_port     = 7946
    protocol    = "tcp"
  }
  ingress {
    cidr_blocks = local.subnets
    from_port   = 7946
    to_port     = 7946
    protocol    = "udp"
  }
  ingress {
    cidr_blocks = local.subnets
    from_port   = 4789
    to_port     = 4789
    protocol    = "udp"
  }

  // Allow all traffic to go out
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

// Enable accessing RDS instance from private subnets
resource "aws_security_group" "db_access" {
  name_prefix = "d8x-cluster-posgtres-access-sg"
  vpc_id      = aws_vpc.d8x_cluster_vpc.id

  tags = {
    Name = "d8x-cluster-posgtres-access-sg"
  }

  ingress {
    cidr_blocks = local.subnets
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
  }

  // Allow all traffic to go out
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
