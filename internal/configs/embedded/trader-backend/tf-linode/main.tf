terraform {
  required_providers {
    linode = {
      source = "linode/linode"
    }
  }
}

# Token must be provided via LINODE_TOKEN env var
provider "linode" {}

resource "linode_instance" "manager" {
  type            = var.worker_size
  region          = var.region
  private_ip      = true
  label           = format("%s-%s", var.server_label_prefix, "manager")
  image           = "linode/ubuntu22.04"
  booted          = true
  authorized_keys = var.authorized_keys

  #   interface {
  #     purpose = "vlan"
  #     label   = "d8x-cluster-vlan"
  #   }
}

resource "linode_instance" "nodes" {
  count = var.num_workers

  type            = var.worker_size
  region          = var.region
  private_ip      = true
  label           = format("%s-%s", var.server_label_prefix, "worker-${count.index + 1}")
  image           = "linode/ubuntu22.04"
  booted          = true
  authorized_keys = var.authorized_keys

  #   interface {
  #     purpose = "vlan"
  #     label   = "d8x-cluster-vlan"
  #   }

}

resource "linode_instance" "broker_server" {
  count           = var.create_broker_server ? 1 : 0
  type            = var.broker_size
  region          = var.region
  private_ip      = true
  label           = format("%s-%s", var.server_label_prefix, "broker-server")
  image           = "linode/ubuntu22.04"
  booted          = true
  authorized_keys = var.authorized_keys

  #   interface {
  #     purpose = "vlan"
  #     label   = "d8x-cluster-vlan"
  #   }

}

# Set up ip permissions for cluster nodes for managed db if cluster id is
# provided
resource "linode_database_access_controls" "pgdb" {
  count         = var.linode_db_cluster_id > 0 ? 1 : 0
  database_id   = var.linode_db_cluster_id
  database_type = "postgresql"
  depends_on    = [linode_instance.manager, linode_instance.nodes]
  allow_list    = concat(linode_instance.nodes.*.private_ip_address, [linode_instance.manager.private_ip_address])
}

# Geneate ansible inventory
resource "local_file" "hosts_cfg" {
  depends_on = [linode_instance.manager, linode_instance.nodes]
  content = templatefile("inventory.tpl",
    {
      manager_public_ip   = linode_instance.manager.ip_address
      manager_private_ip  = linode_instance.manager.private_ip_address
      workers_public_ips  = linode_instance.nodes.*.ip_address
      workers_private_ips = linode_instance.nodes.*.private_ip_address
      broker_public_ip    = var.create_broker_server ? linode_instance.broker_server[0].ip_address : ""
      broker_private_ip   = var.create_broker_server ? linode_instance.broker_server[0].private_ip_address : ""
    }
  )
  filename = "hosts.cfg"
}


# resource "local_file" "pg.cert" {
#     depends_on = [ linode_database_access_controls.pgdb ]
#     content = linode_database_access_controls.pgdb.
# }
