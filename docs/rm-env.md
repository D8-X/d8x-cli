# d8x setup rm-env

Delete an environment's files from the infra repo. Works on both provisioned and non-provisioned envs.

```bash
d8x setup rm-env                  # interactive picker
d8x setup rm-env --env staging    # skip the picker
```

This only edits the infra repo on GitHub. It does NOT call terraform, SSH, or any cloud API. If you also want the servers gone, run `d8x tf-destroy` first.

## What you'll see

1. Pick the environment. Each row shows the env name, chain id, and whether it is currently provisioned, e.g. `staging  (chain 84532)  [provisioned]`.
2. The CLI lists every file it will delete under `<env>/` on the infra repo.
3. If the env is provisioned, a warning reminds you that cloud servers are NOT being touched and points at `tf-destroy`.
4. First confirmation: `Remove the "<env>" environment from the infra repo? [y/N]`. Default `no`.
5. Second confirmation: type the env name exactly. Anything else aborts.
6. The CLI commits the deletion to the infra repo with `remove <env>/ from infra repo`.

## Requirements

- `GITHUB_TOKEN` (Bitwarden or .env) with write access to the infra repo.

## When to use which

- **Created an env by mistake, never provisioned**: `rm-env`.
- **Done with a deployed env**: `tf-destroy` first (tears down the cloud servers), then `rm-env` to also remove the `<env>/` directory from the infra repo.
- **`tf-destroy` already ran, files are still in the infra repo**: `rm-env` to finish the cleanup.

## What is preserved

- Bitwarden entries for the env (e.g. `SSH_KEY_{ENV}`, `SERVER_PASSWORD_{ENV}`, `AWS_*_{ENV}`, `LINODE_TOKEN_{ENV}`). Delete these by hand in Bitwarden if you want them gone.
- Local SSH key files.
- Local terraform state, if any.
