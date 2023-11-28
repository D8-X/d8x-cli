

# # Provision postgres on RDS
resource "aws_db_subnet_group" "pg_subnet" {
  name       = format("%s-%s", var.server_label_prefix, "postgres-subnet")
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
  identifier             = format("%s-%s", var.server_label_prefix, "pg")
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
