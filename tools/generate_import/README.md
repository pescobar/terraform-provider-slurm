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
| `--layout` | — | `flat` |

## Layouts (`--layout`)

The script can emit two layouts. Both import the exact same Slurm state; they
differ only in how the configuration is organised on disk.

### `flat` (default)

One HCL block per resource. Simple and direct, but a single `users.tf` becomes
unwieldy at thousands of users. Generated files:

| File | Contents |
|------|----------|
| `provider.tf` | `terraform {}` block, provider config, token variable |
| `qos.tf` | One `slurm_qos` resource per QOS |
| `accounts.tf` | One `slurm_account` resource per account |
| `users.tf` | One `slurm_user` resource per user with embedded association blocks |
| `imports.tf` | One `import {}` block per resource (QOS → accounts → users) |

### `big-cluster`

Recommended for large clusters. Emits an **account-centric** data layout that
sysadmins edit by hand, plus a write-once `generate.tf` that inverts it into
the user-centric resources the provider needs (see
`examples/big-cluster/README.md` for the daily-ops workflow). Generated files:

| File | Contents |
|------|----------|
| `main.tf` | `terraform {}` block, provider config, token variable |
| `qos.tf` | One `slurm_qos` resource per QOS (plain HCL, as in flat) |
| `generate.tf` | `locals` + `for_each` that build the resources from `data/` |
| `data/accounts/<name>.yaml` | One file per account: metadata + `user_associations` list |
| `data/users.yaml` | Exceptions only: admins and multi-account default picks |
| `imports.tf` | `import {}` blocks targeting the `for_each` addresses (`slurm_account.this["…"]`) |

Notes specific to `big-cluster`:

- **Account membership is inverted** from Slurm's user-centric associations:
  each account file's `user_associations` list carries every user associated
  with it. Per-association data is split across two sub-keys on the object
  form of an entry: `account_overrides` for fields `slurm_account` also has
  (QOS, fairshare, max_jobs, TRES limits — a value here overrides what the
  member would otherwise inherit from the account), and `association` for
  fields with no account-level equivalent (partition, priority, job-count
  and wall-clock limits — declared, not overridden). See "Two kinds of
  per-user data" in `examples/big-cluster/README.md`.
- **`data/users.yaml` stays tiny.** A user is listed only if they have an
  `admin_level` or belong to more than one account (to pin their login
  `default_account`). Single-account users are derived automatically.
- **Filenames are sanitized**, but the real Slurm account name is preserved in
  each file's `name:` key, so accounts whose names contain `/`, spaces, etc.
  are handled correctly.
- **Users with no associations are skipped** (they cannot be represented in an
  account-centric layout) and reported as a warning.
- The workflow (`tofu init` → `apply` → `apply` → `plan`) is identical to flat.

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

`fairshare` is emitted as a **string** (the provider's attribute is a string,
not a number): a decimal weight like `"50"`, or the keyword `"parent"` when the
association is in parent-inheritance mode. Slurm stores `fairshare=parent` as
`shares_raw = 2147483647` (`INT32_MAX` / `SLURMDB_FS_USE_PARENT`); the script
maps that sentinel back to `"parent"`. It is omitted when its value is `≤ 1`
because Slurm's API returns `1` both for "explicitly set to 1" and "never
configured" (the default), so the two cases are indistinguishable. If you need
to explicitly manage `fairshare = "1"`, add it to the generated HCL by hand.

`organization` is omitted when it equals the account's own name: Slurm
defaults `Organization` to the account name when it is not explicitly set
(verified with `sacctmgr show account`), so the two are indistinguishable —
same reasoning as `fairshare` above. If your account's organization
genuinely matches its name, add it to the generated HCL by hand.

### Users

For each user the script emits one `association {}` block per account
association found in Slurm, covering every field the `association` block
accepts: `partition`, `fairshare`, `priority`, `default_qos`, `allowed_qos`,
the job-count limits (`max_jobs`, `max_jobs_accrue`, `max_submit_jobs`,
`grp_jobs`, `grp_jobs_accrue`, `grp_submit_jobs`), the wall-clock limits
(`max_wall_pj`, `grp_wall`), and the TRES limits (`max_tres_per_job`,
`max_tres_per_node`, `max_tres_mins_per_job`, `grp_tres`, `grp_tres_mins`,
`grp_tres_run_mins`) — each emitted only when Slurm returns a non-default
value.

**`allowed_qos`:** emitted only when Slurm returns a non-empty list for
that specific association *and* it differs from the parent account's own
QOS list. Field names in the generated HCL/YAML exactly match the
provider's `slurm_user` `association` block attribute names — this
includes `allowed_qos`, not the older `qos` name some pre-v0.2.1 configs
may still use.

**Inherited-value ambiguity (`default_qos`, `max_jobs`, `max_tres_per_job`,
`max_tres_per_node`, `max_tres_mins_per_job`):** verified against a live
cluster, Slurm's REST API resolves these five association fields to the
account's own effective value when the user's association never set them —
there is no separate flag distinguishing "inherited from the account" from
"explicitly set to the same value". The script therefore omits each of
these fields when it matches the parent account's own value, on the
(usual, and the only sound default given the ambiguity) assumption that a
match means inheritance, not an intentional pin. If a user's association
genuinely must pin one of these fields to a value that happens to equal
the account's current value, add it to the generated HCL/YAML by hand — and
note that a later change to the account's own value will then silently
diverge from that pin, same as any other explicit override.
`grp_tres` / `grp_tres_mins` / `grp_tres_run_mins` do **not** inherit this
way (also verified) and are always emitted as the association's own value.

**`fairshare`:** same rule as accounts — emitted as a string (`"50"` or
`"parent"`), omitted when `≤ 1`.

**`admin_level`:** omitted when `None` (the default); included for `Operator`
and `Administrator`.

**`default_wc_key`:** the user's default workload characterization key. The
script reads it from `user.default.wckey`, matching where the provider
itself reads it — but as a **known Slurm REST API limitation** (verified
against a live 26.05.1 cluster; not something this project can fix),
`default.wckey` is always returned empty on every user/association/wckey
endpoint, even immediately after `sacctmgr` confirms the value is set
server-side. In practice this means the script currently **never emits
`default_wc_key`**, regardless of what is configured in Slurm. If you use
`default_wc_key`, declare it by hand in the generated config — the
provider can still *write* it correctly (verified), it just cannot read it
back to detect drift, the same way `tofu plan` will never flag it either.

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
- **(`--layout big-cluster` only) Users with zero associations are skipped.**
  The account-centric layout represents users as entries in some account's
  `user_associations` list, so a user with no associations has nowhere to
  live and is omitted (reported as a warning listing the skipped names). In
  practice such users can't run jobs, so this is usually harmless. If you
  must manage association-less users, they would need to be added to
  `data/users.yaml` and `generate.tf` extended to create association-less
  `slurm_user` resources from those entries — this is not done
  automatically. The `flat` layout is unaffected (it emits every user).
