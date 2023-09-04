variable "num_workers" {
  type        = number
  description = "Number of worker nodes to create"
  default     = 3
}

variable "region" {
  type        = string
  description = "Cluster region"
  default     = "eu-central"
}

variable "worker_size" {
  type        = string
  description = "Worker linodes size"
  default     = "g6-standard-2"
}

variable "broker_size" {
  type        = string
  description = "Worker linodes size"
  default     = "g6-standard-1"
}

variable "authorized_keys" {
  type        = list(string)
  description = "List of ssh public keys that will be added to each server"
}

variable "linode_db_cluster_id" {
  type        = number
  description = "Linode postgres database cluster id (must be already created)"
  default     = -1
}

variable "create_broker_server" {
  type        = bool
  description = "Whether broker-server node should be created"
  default     = true
}

variable "server_label_prefix" {
  type        = string
  description = "Prefix that will be used in server label's"
  default     = "d8x-cluster"
}
