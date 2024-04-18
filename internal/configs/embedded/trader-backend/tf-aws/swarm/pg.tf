
// Aditional private subnet in different az for rds
resource "aws_subnet" "private_subnet_2" {
  vpc_id     = var.vpc_id
  cidr_block = var.subnets[2]
  tags = {
    Name = "${var.server_label_prefix}-subnet_private_2"
  }
  availability_zone = "${var.region}b"
}

resource "aws_db_subnet_group" "pg_subnet" {
  name       = format("%s-%s", var.server_label_prefix, "postgres-subnet")
  subnet_ids = [var.workers_subnet_id, aws_subnet.private_subnet_2.id]
  tags = {
    Name = "private subnet association"
  }
}

resource "random_password" "db_password" {
  length  = 28
  special = false
}


// Enable accessing RDS instance from private subnets
resource "aws_security_group" "db_access" {
  name_prefix = "${var.server_label_prefix}-posgtres-access-sg"
  vpc_id      = var.vpc_id

  tags = {
    Name = "${var.server_label_prefix}-posgtres-access-sg"
  }

  ingress {
    cidr_blocks = var.subnets
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

// Create RDS instance
resource "aws_db_instance" "pg" {
  identifier             = format("%s-%s", var.server_label_prefix, "pg")
  instance_class         = var.db_instance_class
  allocated_storage      = 5
  engine                 = "postgres"
  engine_version         = "15.4"
  username               = "d8xtrader"
  password               = random_password.db_password.result
  vpc_security_group_ids = [aws_security_group.db_access.id]
  db_subnet_group_name   = aws_db_subnet_group.pg_subnet.name
  publicly_accessible    = false
  skip_final_snapshot    = true
  max_allocated_storage  = 50
}

// Database credentials file structure
variable "pg_details" {
  default = <<EOF
host: %s
user: %s
port: %s
password: %s
  EOF
}

// Create the credentials file
resource "local_file" "rds_db_password" {
  depends_on = [aws_db_instance.pg]
  content    = format(var.pg_details, aws_db_instance.pg.address, aws_db_instance.pg.username, aws_db_instance.pg.port, random_password.db_password.result)
  // Outside terraform dir
  filename = "../${var.rds_creds_filepath}"
}


