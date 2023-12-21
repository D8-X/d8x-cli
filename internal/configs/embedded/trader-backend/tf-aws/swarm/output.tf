output "manager" {
  value = aws_instance.manager
}

output "workers" {
  value = aws_instance.nodes
}
