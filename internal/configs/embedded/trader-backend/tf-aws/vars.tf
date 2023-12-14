
// All our cidrs will be defined with this variable
locals {
  subnets = cidrsubnets("10.0.0.0/20", 4, 4, 4)
}

variable "aws_access_key" {
  type        = string
  description = "AWS access key value"
  sensitive   = true
}

variable "aws_secret_key" {
  type        = string
  description = "AWS secret key value"
  sensitive   = true
}

variable "num_workers" {
  type        = number
  description = "Number of worker nodes to create"
  default     = 4
}

variable "region" {
  type        = string
  description = "Cluster region"
}

variable "worker_size" {
  type        = string
  description = "Worker EC2 instance size"
  default     = "t3.small"
}

variable "server_label_prefix" {
  type        = string
  description = "Prefix that will be used in instance tag"
  default     = "d8x-cluster"
}

variable "authorized_key" {
  type        = string
  description = "SSH public key that will be added to each server"
}

variable "db_instance_class" {
  type        = string
  description = "Postgres database instance size"
}

variable "rds_creds_filepath" {
  type        = string
  description = "RDS Postgres database credentials file path"
}

variable "create_broker_server" {
  type        = bool
  description = "Whether broker-server node should be created"
  default     = false
}
variable "create_swarm" {
  type        = bool
  description = "Whether swarm setup should be created"
  default     = true
}
