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

variable "keypair_name" {
  type        = string
  description = "AWS keypair to be used for manager and workers"
}




