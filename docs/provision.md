# d8x setup provision

Provisions servers on Linode or AWS using Terraform.

## What it does

1. Selects environment
2. Fetches `terraform/{provider}/*.tf` from infra repo
3. Fetches `{env}/terraform.tfvars` from infra repo
4. Runs `terraform init` + `terraform apply` — creates servers
5. Pushes generated `hosts.cfg` to `{env}/hosts.cfg` in infra repo
6. Saves SSH key to Bitwarden as `SSH_KEY_{ENV}`

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)
- `LINODE_TOKEN` or `AWS_ACCESS_KEY`/`AWS_SECRET_KEY` (Bitwarden, .env, or prompted)

## After this

Create DNS A records for all domains pointing to the manager IP (visible in `hosts.cfg`). Then proceed to `d8x setup configure`.
