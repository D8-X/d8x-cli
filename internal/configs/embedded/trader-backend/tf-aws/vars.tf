

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
  default     = "eu-north-1"
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

variable "ami_image_id" {
  type        = string
  description = "value of the AMI image id to be used for all servers"
  # Ubuntu LTS 22.04
  default = "ami-0989fb15ce71ba39e"
}

variable "authorized_key" {
  type        = string
  description = "SSH public key that will be added to each server. A "
}
