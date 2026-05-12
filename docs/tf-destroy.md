# d8x tf-destroy

Destroy all servers and infrastructure created for an environment. Irreversible.

```bash
d8x tf-destroy
```

## What you'll see

1. Pick the environment.
2. A banner lists what will be destroyed: provider, region, label prefix, manager / worker / broker IPs, and which components are currently deployed.
3. First confirmation: `Proceed with destroying "<env>"? [y/N]`. Choose `no` to abort.
4. Second confirmation: type the environment name exactly. Anything else aborts.
5. The CLI fetches the terraform configs and `terraform.tfvars` for the env from the infra repo, runs `terraform init`, then `terraform destroy --auto-approve`.
6. On success, the CLI prints `Environment "<env>" successfully destroyed:` followed by the same summary that was shown in step 2.
7. Final prompt: `Proceed with bookkeeping cleanup ...? [Y/n]`. Default `yes`.

## After destroying

If you confirm the cleanup prompt, the CLI clears the deployment flags on the env, pushes the cleared config back to `<env>/config.json` in the infra repo, deletes the local `./hosts.cfg`, and deletes `<env>/hosts.cfg` from the infra repo. Each step is soft-failing, so the cleanup is reported as successful even if a sync step warns.

If you decline the cleanup prompt, the cloud resources are gone but `<env>/config.json` and `<env>/hosts.cfg` on the infra repo still describe the pre-destroy state, and your local `./hosts.cfg` is intact. Rerun `d8x tf-destroy` later to clean them up (terraform will report nothing to destroy, and you'll get the same cleanup prompt again).

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env)
- For AWS: `AWS_ACCESS_KEY_{ENV}` and `AWS_SECRET_KEY_{ENV}` in Bitwarden
- For Linode: `LINODE_TOKEN_{ENV}` in Bitwarden
- A local `./terraform/terraform.tfstate` file from the original provision. Terraform uses local state, so destroy must run on a machine that has the state file. If you lost it, terraform has no record of what to destroy, and resources must be cleaned up manually in the cloud console.

## When things go wrong

- **Wrong environment selected**: hit `no` at the first prompt, or type a non-matching name at the second.
- **Terraform init or destroy fails**: rerun the command. It is idempotent. Terraform tracks what survived, and the CLI's pre-fetch step is safe to repeat.
- **GitHub sync warnings at the end** (`could not publish reset state ...`, `could not delete <env>/hosts.cfg`): the cloud resources have already been destroyed. The warnings only affect bookkeeping in the infra repo. Either push the cleared state by rerunning the command later, or fix the file by hand on GitHub.

## What is preserved

`tf-destroy` only cleans up state that is specific to the destroyed environment. Anything reused across environments is left alone.

Kept on purpose:

- The env's directory in the infra repo: `<env>/config.json` (deployment flags cleared but file kept), `<env>/terraform.tfvars`, `<env>/sites.conf`, etc.
- All Bitwarden entries, including cross-env ones like `GITHUB_TOKEN`, `NGINX_API_KEY`, and per-env ones like `AWS_ACCESS_KEY_{ENV}`, `LINODE_TOKEN_{ENV}`, `SSH_KEY_{ENV}`, `SERVER_PASSWORD_{ENV}`.
- Your local SSH key files.

You can re-provision the same environment with `d8x setup provision` and reuse the same credentials.

Cleaned up:

- `<env>/hosts.cfg` on the infra repo (it described servers that no longer exist).
- `./hosts.cfg` on your local machine (same reason).
- The `deployed`/`*_deployed` flags inside `<env>/config.json` (reset to `false`).
