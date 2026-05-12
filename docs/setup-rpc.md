# d8x setup rpc

Add or remove RPC URLs on a live cluster, with a rolling restart of the affected services.

```bash
d8x setup rpc
```

## What you'll see

1. Pick the environment.
2. The CLI prints the current `api` and `history` RPC pools (HTTP and WS) for the chain.
3. Menu options: `Add HTTP RPC`, `Add WS RPC`, `Delete HTTP RPCs`, `Delete WS RPCs`, `Apply and deploy`, `Cancel without saving`.
4. When you Add or Delete, you'll be asked whether it targets `api only`, `history only`, or `both`.
5. `Apply and deploy` shows a per-service diff and asks for confirmation. Choose `no` to abort with no remote changes.

## After confirming

The CLI backs up the existing RPC files on the manager, writes the new ones, rolls `api` and `history` onto the new config, and probes their public hostnames. The revision id printed at the end (e.g. `cfg_rpc_20260512113224`) matches the backup filenames.

## When things go wrong

- **Apply was refused** because `api HTTP` or `history HTTP` is empty: add at least one HTTP URL before applying.
- **Service rollout failed mid-apply**: the new JSON is on disk but the service may still be running the previous config. Rerunning the command is safe and idempotent.
- **Undo your last change**: rerun `d8x setup rpc` and use the menu to remove what you just added (or re-add what you deleted). Apply rolls the services onto the corrected list.
- **Restore from backups on the manager** (revert to the file content from before an apply): SSH into the manager and copy the `.bak.<rev>` files back into place.
  ```bash
  d8x ssh manager
  cp -p trader-backend/rpc.main.json.bak.<rev>    trader-backend/rpc.main.json
  cp -p trader-backend/rpc.history.json.bak.<rev> trader-backend/rpc.history.json
  exit
  ```
  Then rerun `d8x setup rpc` and pick `Apply and deploy` without editing anything. The CLI will detect that the manager's files already match the pool, then ask whether to reroll the services anyway. Confirm to push the restored content onto the running services.

## Notes

- Only `api` and `history` are touched. Candles and broker keep their own RPC files.
- Requires the cluster to have been deployed with `d8x setup swarm-deploy`.
