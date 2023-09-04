[managers]
${manager_public_ip} manager_private_ip=${manager_private_ip} hostname=manager-1

[workers]
%{ for index, ip in workers_public_ips ~}
${ip} hostname=${format("worker-%02d", index + 1)}
%{ endfor ~}

[broker]
${broker_public_ip} private_ip=${broker_private_ip}
