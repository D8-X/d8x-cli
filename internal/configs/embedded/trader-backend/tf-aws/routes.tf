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

// Associate public subnets with public route table for internet access through
// igw
resource "aws_route_table_association" "public_internet" {
  subnet_id      = aws_subnet.public_subnet.id
  route_table_id = aws_route_table.public.id
}
