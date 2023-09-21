

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
  description = "SSH public key that will be added to each server. A "
}


variable "create_broker_server" {
  type        = bool
  description = "Whether broker-server node should be created"
  default     = false
}
