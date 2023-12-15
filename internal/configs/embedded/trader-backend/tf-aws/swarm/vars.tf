variable "subnets" {
  type        = list(string)
  description = "Subnets of the VPC"
}


variable "server_label_prefix" {
  type        = string
  description = "Prefix that will be used in instance tag names"
}

variable "vpc_id" {
  type        = string
  description = "VPC id where swarm resources will be created"
}

variable "ami_image_id" {
  type        = string
  description = "AWS AMI image id which will be used for instances"
}

variable "manager_instance_type" {
  type        = string
  description = "Manager instance type"
}

variable "worker_instance_type" {
  type        = string
  description = "Worker instance type"
}

variable "num_workers" {
  type        = number
  description = "Number of worker nodes to create"
}

variable "public_subnet_id" {
  type        = string
  description = "Public subnet id"
}

variable "workers_subnet_id" {
  type        = string
  description = "Workers subnet id"
}

variable "security_group_ids_manager" {
  type        = list(string)
  description = "List of security group ids for swarm manager"
}
variable "security_group_ids_workers" {
  type        = list(string)
  description = "List of security group ids for swarm workers"
}

variable "keypair_name" {
  type        = any
  description = "AWS keypair to be used for manager and workers"
}

variable "db_instance_class" {
  type        = string
  description = "Postgres database instance size"
}

variable "rds_creds_filepath" {
  type        = string
  description = "RDS Postgres database credentials file path"
}

variable "region" {
  type        = string
  description = "Cluster region"
}
