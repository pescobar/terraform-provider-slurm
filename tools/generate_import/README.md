# generate_import.py

Queries a running `slurmrestd` instance and generates OpenTofu HCL
configuration files plus `import {}` blocks for all existing Slurm
accounting resources (QOS, accounts, users with their associations).

## Requirements

- Python 3.10+ (uses `match`-free walrus operators; stdlib only, no `pip install`)
- OpenTofu 1.6+ (for native `import {}` block support)
- A running `slurmrestd` reachable from the machine where the script runs
- A valid JWT token (`scontrol token lifespan=3600`)

## Usage

```bash
python3 tools/generate_import/generate_import.py \
  --endpoint  http://localhost:6820 \
  --token     "$SLURM_JWT_TOKEN" \
  --cluster   linux \
  --api-version v0.0.42 \
  --output-dir ./generated
```

All flags can be provided via environment variables instead:

| Flag | Environment variable | Default |
|------|---------------------|---------|
| `--endpoint` | `SLURM_REST_URL` | `http://localhost:6820` |
| `--token` | `SLURM_JWT_TOKEN` | _(required)_ |
| `--cluster` | `SLURM_CLUSTER` | `linux` |
| `--api-version` | `SLURM_API_VERSION` | `v0.0.42` |
| `--output-dir` | — | `./generated` |

## Generated files

| File | Contents |
|------|----------|
| `provider.tf` | `terraform {}` block, provider config, token variable |
| `qos.tf` | One `slurm_qos` resource per QOS |
| `accounts.tf` | One `slurm_account` resource per account |
| `users.tf` | One `slurm_user` resource per user with embedded association blocks |
| `imports.tf` | One `import {}` block per resource (QOS → accounts → users) |

## Import workflow

```bash
cd generated/

# 1. Initialise OpenTofu (downloads the provider binary)
tofu init

# 2. First apply — import blocks pull Slurm state into OpenTofu state.
#    Pass the same token you used when running the script.
tofu apply -var="slurm_token=$SLURM_JWT_TOKEN"

# 3. Reconcile apply — writes config-declared values back to Slurm for
#    fields that were left null by the import (null-preservation pattern).
tofu apply -var="slurm_token=$SLURM_JWT_TOKEN"

# 4. Verify — plan must show no changes.
tofu plan -detailed-exitcode -var="slurm_token=$SLURM_JWT_TOKEN"
```

After step 4 the state and Slurm are in sync. You can move the generated
files into your own Terraform project and delete `imports.tf` — it is only
needed once.

## What the script generates (and what it skips)

### QOS

All non-zero / non-default field values are emitted. The `normal` QOS (Slurm's
built-in) is included with a warning comment because it must **not** be
destroyed via `tofu destroy` — doing so corrupts slurmdbd. To stop managing
it without deleting it:

```bash
tofu state rm slurm_qos.normal
```

### Accounts

All fields are emitted when set. The `root` account is always skipped — it
is Slurm's built-in top-level account and is not managed by this provider.

Child accounts include a `depends_on` referencing their parent so OpenTofu
creates and imports them in the correct order.

`fairshare` is omitted when its value is `≤ 1` because Slurm's API returns
`1` both for "explicitly set to 1" and "never configured" (the default), so
the two cases are indistinguishable. If you need to explicitly manage
`fairshare = 1`, add it to the generated HCL by hand.

### Users

For each user the script emits one `association {}` block per account
association found in Slurm.

**QOS list (`qos`):** emitted only when Slurm returns a non-empty list for
that specific association. An empty list means the user inherits the QOS list
from the parent association up the hierarchy, so no `qos` block is needed.

**`fairshare`:** same rule as accounts — omitted when `≤ 1`.

**`admin_level`:** omitted when `None` (the default); included for `Operator`
and `Administrator`.

### Import null-preservation

`slurm_account` and `slurm_user` use an **Optional-only** schema: fields are
only tracked in state when they are non-null in config. After the first
`tofu apply` (which runs the `import {}` blocks) most association limit fields
will be null in state. The second `tofu apply` (reconcile) writes the
config-declared values to Slurm and populates state. After that,
`tofu plan` must be clean.

`slurm_qos` behaves differently: all fields are `Optional + Computed`, so
after import the provider reads them from Slurm and state is immediately
correct. No reconcile apply is needed for QOS resources.

See `docs/resources/user.md` and `docs/resources/account.md` for more detail
on the null-preservation pattern.

## Name sanitization

Terraform resource labels must match `[a-zA-Z0-9_]` and cannot start with a
digit. The script applies these rules:

1. Replace any character outside `[a-zA-Z0-9_]` with `_`
   (hyphens, dots, spaces, etc.)
2. If the result starts with a digit, prepend the resource type:
   a QOS named `6hours` becomes `slurm_qos.qos_6hours`.

If two Slurm names produce the same Terraform label the script aborts with
a clear error. Rename one of the conflicting resources in Slurm before
re-running.

## Limitations

- **Single-cluster only.** The script scopes all association queries to the
  cluster passed via `--cluster`. Multi-cluster setups require running the
  script once per cluster and merging the output.
- **`fairshare = 1` is silently omitted.** See the accounts and users
  sections above.
- **Partition-scoped associations** are supported: if a user has an
  association scoped to a specific partition, the `partition` field is
  included in the `association {}` block.
