# system-qos-warning

Plan-time fixture covering the `slurm_qos` `ValidateConfig` warning that
fires when a user manages one of Slurm's auto-created system QOS rows
(currently `normal`). Documented as Bug 3 in the project root `CLAUDE.md`:
managing the row works on the first apply but the destroy soft-deletes
it (`deleted=1`), and the next apply fails with `"Slurmdbd query
returned with empty list"` because slurmrestd's verification query does
not match the restored system row.

## Run

```bash
cd test/fixtures/system-qos-warning
TOKEN=$(docker exec slurmctld scontrol token lifespan=600 | sed 's/SLURM_JWT=//')
tofu plan -var "slurm_token=$TOKEN"
```

## Pass criteria

- `tofu plan` **exits 0** — the diagnostic is a warning, not an error,
  so it does not block plan.
- Output contains `Warning: Managing built-in system QOS "normal" is
  fragile`, anchored to the `name` attribute of `slurm_qos.system_normal`.

This fixture intentionally does not run `tofu apply`. Applying it would
silently mutate the live database's system QOS row — which is the
footgun the warning is meant to prevent.
