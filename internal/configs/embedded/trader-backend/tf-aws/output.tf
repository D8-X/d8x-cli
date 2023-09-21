output "manager_ip" {
  description = "public ip address of manager node"
  value       = aws_instance.manager.public_ip
}

output "manager_private_ip" {
  description = "private ip address of manager node"
  value       = aws_instance.manager.private_ip
}

output "nodes_private_ips" {
  description = "private ip addresses of worker nodes"
  value       = aws_instance.nodes.*.private_ip
}
