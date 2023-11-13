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


resource "aws_security_group" "http_access" {
  name_prefix = "d8x-cluster-http-sg"
  vpc_id      = aws_vpc.d8x_cluster_vpc.id

  tags = {
    Name = "d8x-cluster-http-sg"
  }

  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
  }

  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}


// nfs server access from nodes to manager
resource "aws_security_group" "nfs_access" {
  name_prefix = "d8x-cluster-nfs-sg"
  vpc_id      = aws_vpc.d8x_cluster_vpc.id

  tags = {
    Name = "d8x-cluster-nfs-sg"
  }

  ingress {
    cidr_blocks = local.subnets
    from_port   = 2049
    to_port     = 2049
    protocol    = "tcp"
  }

  ingress {
    cidr_blocks = local.subnets
    from_port   = 2049
    to_port     = 2049
    protocol    = "udp"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "cadvisor_port" {
  name_prefix = "d8x-cluster-nfs-sg"
  vpc_id      = aws_vpc.d8x_cluster_vpc.id

  tags = {
    Name = "d8x-cluster-cadvisor-sg"
  }

  ingress {
    cidr_blocks = local.subnets
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

