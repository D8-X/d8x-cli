// Route table for public subnets (manager and broker)
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.d8x_cluster_vpc.id

  tags = {
    Name = "d8x-cluster-public-route-table"
  }
}

resource "aws_route" "internet" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.d8x_igw.id
}

resource "aws_route_table_association" "manager_association" {
  subnet_id      = aws_subnet.public_subnet.id
  route_table_id = aws_route_table.public.id
}

// Configure internet access for private subnet (for worker nodes)
resource "aws_route_table" "workers_internet" {
  vpc_id = aws_vpc.d8x_cluster_vpc.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.public_nat.id
  }

  tags = {
    Name = "d8x-cluster-private-route-table"
  }
}

resource "aws_route_table_association" "workers_internet" {
  subnet_id      = aws_subnet.workers_subnet.id
  route_table_id = aws_route_table.workers_internet.id
}
