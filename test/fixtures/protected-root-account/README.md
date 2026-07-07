# protected-root-account

Plan-time fixture covering the `slurm_account` `ModifyPlan` guard that
refuses to plan a deletion of Slurm's built-in `root` account. See
`protectedAccountNames` / `protectedAccountDeleteError` in
`internal/resources/account.go`, and the matching discussion in
`examples/big-cluster/README.md`.

Unlike the `normal` system QOS (Bug 3 in the project root `CLAUDE.md`,
`test/fixtures/system-qos-warning/`), there is no valid Slurm workflow that
deletes just the root account while the cluster stays registered — so this
is an unconditional **error**, not a warning, and it fires at `tofu plan`
time (via `ModifyPlan`), before Delete ever runs.

## Run

```bash
cd test/fixtures/protected-root-account
TOKEN=$(docker exec slurmctld scontrol token lifespan=600 | sed 's/SLURM_JWT=//')

# 1. Import Slurm's existing root account. Read-only -- does not create or
#    modify anything. Every optional attribute is left unset in main.tf, so
#    this should be followed by a clean `tofu plan` (no diff).
tofu import -var "slurm_token=$TOKEN" slurm_account.root_protected root
tofu plan -var "slurm_token=$TOKEN"

# 2. Outright destroy: simulates what `tofu destroy` would plan, without
#    editing config or touching the cluster (plan is always read-only).
tofu plan -destroy -var "slurm_token=$TOKEN"

# 3. Rename-triggered replace: `name` has RequiresReplace(), so changing it
#    plans a destroy of the old ("root") instance + create of a new one --
#    just as much a deletion of the protected account as step 2, even though
#    the planned value isn't null. Revert the file afterwards.
sed -i 's/name = "root"/name = "root_renamed"/' main.tf
tofu plan -var "slurm_token=$TOKEN"
git checkout main.tf
```

## Pass criteria

- Step 1: `tofu plan` **exits 0** with no planned changes.
- Step 2 and step 3: `tofu plan` **exits non-zero**, and the output contains
  `Error: Refusing to delete protected account "root"`.
- This fixture never runs `tofu apply` and never runs a real
  `tofu destroy` — every check above only exercises `tofu plan`, which does
  not call the provider's `Delete` function.
