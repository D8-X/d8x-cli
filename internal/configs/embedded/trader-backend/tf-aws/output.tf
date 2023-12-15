# output "manager_ip" {
#   description = "public ip address of manager node"
#   value       = module.swarm_servers[0].manager.public_ip
# }

# output "manager_private_ip" {
#   description = "private ip address of manager node"
#   value       = module.swarm_servers[0].manager.private_ip
# }

# output "nodes_private_ips" {
#   description = "private ip addresses of worker nodes"
#   value       = module.swarm_servers[0].workers.*.private_ip
# }
