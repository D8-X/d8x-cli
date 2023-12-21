

// Configure internet access for private subnet (for worker nodes)
resource "aws_route_table" "workers_internet" {
  vpc_id = var.vpc_id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.public_nat.id
  }

  tags = {
    Name = "${var.server_label_prefix}-private-route-table"
  }
}

resource "aws_route_table_association" "workers_internet" {
  subnet_id      = var.workers_subnet_id
  route_table_id = aws_route_table.workers_internet.id
}
