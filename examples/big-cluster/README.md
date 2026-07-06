# Big-cluster example — human-friendly, account-centric layout

Managing hundreds of accounts and thousands of users as one giant `users.tf` is
painful. This example shows a maintainable alternative: sysadmins edit small,
**account-centric** YAML files, and a thin, write-once HCL layer inverts that
data into the **user-centric** resources the Slurm provider needs.

> **Importing an existing cluster?** Don't hand-write these files. Run the
> importer with `--layout big-cluster` to generate this exact structure
> (plus `imports.tf`) from a live cluster:
> `python3 tools/generate_import/generate_import.py --layout big-cluster …`.
> See `tools/generate_import/README.md`. The files below are a hand-written
> illustration of what it produces.

## Why invert?

A `slurm_user` is a single resource that carries **all** of its associations
(embedded `association` blocks). A user in 6 accounts is *one* resource
referencing 6 accounts — you can't split that block across 6 files in raw HCL.

So we store data the way humans think ("who is in `lab_physics`?") and let
`generate.tf` assemble the user resources.

## Layout

```
big-cluster/
├── main.tf         # provider config
├── qos.tf          # a few QOS as plain HCL (change rarely)
├── generate.tf     # locals + for_each — WRITE ONCE, rarely touched
└── data/           # <-- the only files sysadmins edit day to day
    ├── accounts/
    │   ├── lab_physics.yaml   # one file per account: metadata + user_associations
    │   ├── lab_bio.yaml
    │   ├── lab_chem.yaml
    │   ├── teaching.yaml
    │   ├── admin.yaml
    │   └── shared.yaml
    └── users.yaml            # EXCEPTIONS ONLY (admins + multi-account defaults)
```

## Account file format (`data/accounts/<name>.yaml`)

```yaml
description: "Physics Lab"
organization: physics
fairshare: 100
default_qos: standard
allowed_qos: [standard, high, long, gpu]
user_associations:
  - alice                 # simple: bare username, no overrides
  - user: john             # object form, only when this user's association
                            # needs account_overrides and/or association data
    account_overrides:
      default_qos: debug
      allowed_qos: [debug, standard]
```

The account **file name** (minus `.yaml`) is the Slurm account name, unless
overridden by a `name:` key (see "Sanitized filenames" below).

### Two kinds of per-user data, and why they're separate keys

Each entry in `user_associations` is a Slurm **association** — the
account+user (+partition) pairing that carries a user's actual limits inside
that account. An association's attributes split into two genuinely different
categories, and the object form gives each its own sub-key so the distinction
is visible in the YAML itself, not just in prose:

- **`account_overrides`** — fields that `slurm_account` *also* has
  (`fairshare`, `default_qos`, `allowed_qos`, `max_jobs`, and the 6 TRES
  fields). For most of these, the account sets a value that every member
  inherits by default, and a value here **overrides** that inherited value
  for this one user — but not all 10 keys inherit; see "What an omitted
  `account_overrides` key resolves to" below for the exact split.
- **`association`** — fields that exist **only** at the association level
  (`partition`, `priority`, the job-count limits, `max_wall_pj`, `grp_wall`).
  `slurm_account` has no matching attribute for any of these — there is
  nothing to inherit or override, you're simply **declaring** a fact about
  this specific user's membership in this specific account.

If you're ever unsure which sub-key a field belongs in: check the "Supported
account-level fields" table below. If the field is in that table, it goes
under `account_overrides`; if it isn't, it goes under `association`.

### Supported account-level fields — complete reference

`generate.tf` passes every one of these through to `slurm_account` verbatim
if present in the account YAML; **every key below is optional** except
`user_associations` (which `generate.tf` always expects, even if empty:
`user_associations: []`). The table matches the `slurm_account` resource
schema field-for-field — if a key isn't in this list, `slurm_account` doesn't
accept it, and it can never appear under a member's `account_overrides`
either. This is a plain transcription of the `Optional` section of
[`docs/resources/account.md`](../../docs/resources/account.md#optional) (the
generated, authoritative schema reference) — that page's
[Null-preservation after import](../../docs/resources/account.md#null-preservation-after-import)
and
[`parent_account` drift is not detected once set](../../docs/resources/account.md#parent_account-drift-is-not-detected-once-set)
sections also apply here unchanged, since these YAML files ultimately produce
ordinary `slurm_account` resources.

| YAML key | `slurm_account` attribute | Slurm name |
|----------|---------------------------|------------|
| `description` | `description` | Description |
| `organization` | `organization` | Organization |
| `parent_account` | `parent_account` | ParentName — see the **ordering caveat** in [Notes](#notes) before using this in a Terraform-managed hierarchy |
| `fairshare` | `fairshare` | Fairshare (the account's own association) |
| `default_qos` | `default_qos` | DefaultQOS (the account's own association) |
| `allowed_qos` | `allowed_qos` | QOS — list of names the account (and, unless overridden, its members) may use |
| `max_jobs` | `max_jobs` | MaxJobs — max concurrently running jobs for the account's own association |
| `max_tres_per_job` | `max_tres_per_job` | MaxTRES — max TRES per job |
| `max_tres_per_node` | `max_tres_per_node` | MaxTRESPerNode — max TRES per node per job |
| `max_tres_mins_per_job` | `max_tres_mins_per_job` | MaxTRESMins — max TRES-minutes per job |
| `grp_tres` | `grp_tres` | GrpTRES — max TRES in use at once across the whole account group |
| `grp_tres_mins` | `grp_tres_mins` | GrpTRESMins — max TRES-minutes across the group |
| `grp_tres_run_mins` | `grp_tres_run_mins` | GrpTRESRunMins — max TRES-minutes of currently running jobs across the group |

TRES fields (any key ending in `_tres*`) use a list-of-objects shape: `type`
(e.g. `cpu`, `mem`, `gres`), optional `name` for generic resources like
`gres` (e.g. `gpu`), and `count`. These same 10 keys (plus `fairshare`,
`default_qos`, `allowed_qos`, `max_jobs`) are exactly the set that's also
valid under a member's `account_overrides:` — see the next section.

Full worked example using every account-level key at once (not a real
account — see `data/accounts/lab_physics.yaml`, `teaching.yaml`, and
`lab_bio.yaml` for realistic, partial, live-tested uses of these fields):

```yaml
description: "Reference account — every account-level field in one place"
organization: physics
# parent_account: some_other_account   # see the ordering caveat before using this
fairshare: 100
default_qos: standard
allowed_qos: [standard, high, long, gpu]
max_jobs: 200
max_tres_per_job:
  - type: cpu
    count: 64
  - type: gres
    name: gpu
    count: 8
max_tres_per_node:
  - type: cpu
    count: 32
max_tres_mins_per_job:
  - type: cpu
    count: 10000
grp_tres:
  - type: cpu
    count: 1000
grp_tres_mins:
  - type: cpu
    count: 200000
grp_tres_run_mins:
  - type: cpu
    count: 50000
user_associations:
  - alice
```

### Supported per-user fields (object form) — complete reference

The object form of an entry (`- user: <name>` plus `account_overrides:`
and/or `association:`) accepts **every field** `slurm_user`'s `association`
block accepts, scoped to that one user's association with that one account —
split across the two sub-keys as described above. Together the two tables
below are a plain transcription of the
[`association` block's nested schema](../../docs/resources/user.md#nestedblock--association)
in [`docs/resources/user.md`](../../docs/resources/user.md) (the generated,
authoritative reference) — minus `account`, which that schema requires
explicitly but which is implicit here (see
["What this layout cannot express"](#what-this-layout-cannot-express) below).
That page's
[why `allowed_qos` is not read during import](../../docs/resources/user.md#why-allowed_qos-is-not-read-during-import)
and
[what to declare in config before importing](../../docs/resources/user.md#what-to-declare-in-config-before-importing)
sections apply here unchanged too.

**`account_overrides`** — same 10 keys as the account-level table, same
meaning, just scoped to one user:

| YAML key | `association` attribute | Slurm name |
|----------|--------------------------|------------|
| `fairshare` | `fairshare` | Fairshare |
| `default_qos` | `default_qos` | DefaultQOS |
| `allowed_qos` | `allowed_qos` | QOS — this user's own list; overrides the account's `allowed_qos` for this membership |
| `max_jobs` | `max_jobs` | MaxJobs |
| `max_tres_per_job` | `max_tres_per_job` | MaxTRES |
| `max_tres_per_node` | `max_tres_per_node` | MaxTRESPerNode |
| `max_tres_mins_per_job` | `max_tres_mins_per_job` | MaxTRESMins |
| `grp_tres` | `grp_tres` | GrpTRES |
| `grp_tres_mins` | `grp_tres_mins` | GrpTRESMins |
| `grp_tres_run_mins` | `grp_tres_run_mins` | GrpTRESRunMins |

**`association`** — the 9 fields with no account-level equivalent; declared,
never "overridden":

| YAML key | `association` attribute | Slurm name |
|----------|--------------------------|------------|
| `partition` | `partition` | Partition — scopes the association to one partition |
| `priority` | `priority` | Association-level priority (distinct from QOS priority) |
| `max_jobs_accrue` | `max_jobs_accrue` | MaxJobsAccrue |
| `max_submit_jobs` | `max_submit_jobs` | MaxSubmitJobs |
| `max_wall_pj` | `max_wall_pj` | MaxWallDurationPerJob (minutes) |
| `grp_jobs` | `grp_jobs` | GrpJobs |
| `grp_jobs_accrue` | `grp_jobs_accrue` | GrpJobsAccrue |
| `grp_submit_jobs` | `grp_submit_jobs` | GrpSubmitJobs |
| `grp_wall` | `grp_wall` | GrpWall (minutes) |

Full worked example using every per-user key at once (not a real member —
see `data/accounts/teaching.yaml` (`dave`), `lab_bio.yaml` (`bob`),
`lab_physics.yaml` (`john`), and `admin.yaml` (`john`) for realistic,
partial, live-tested uses of these fields):

```yaml
user_associations:
  - alice                       # bare form: no overrides
  - user: bob                   # reference member — every key at once
    account_overrides:
      fairshare: 15
      default_qos: debug
      allowed_qos: [debug, standard]
      max_jobs: 5
      max_tres_per_job:
        - type: gres
          name: gpu
          count: 2
      max_tres_per_node:
        - type: cpu
          count: 8
      max_tres_mins_per_job:
        - type: cpu
          count: 1000
      grp_tres:
        - type: cpu
          count: 100
      grp_tres_mins:
        - type: cpu
          count: 20000
      grp_tres_run_mins:
        - type: cpu
          count: 5000
    association:
      partition: cpu
      priority: 20
      max_jobs_accrue: 3
      max_submit_jobs: 10
      max_wall_pj: 240
      grp_jobs: 4
      grp_jobs_accrue: 2
      grp_submit_jobs: 8
      grp_wall: 2000
```

What an omitted `account_overrides` key resolves to at the association level
depends on the field, and it's not the same answer for all of them:

- `default_qos`, `max_jobs`, `max_tres_per_job`, `max_tres_per_node`,
  `max_tres_mins_per_job`, and `allowed_qos` — omitting these means the
  association **inherits the account's own value** for that field (this is
  the "every member inherits by default" behavior described above).
- `fairshare` — omitting it does **not** inherit the account's fairshare;
  Slurm falls back to its own fixed default (`1`) regardless of what the
  account is set to.
- `grp_tres`, `grp_tres_mins`, `grp_tres_run_mins` — these also do **not**
  inherit the account's value when omitted; Slurm falls back to its own
  (unlimited) default for the association.

Set a field explicitly under `account_overrides` any time you want the
member's association to match the account's current value for `fairshare`
or the `grp_tres*` fields — for the other six, omitting it already gets you
the account's value, by inheritance.

See the **inherited-value ambiguity** note in `tools/generate_import/README.md`
if you plan to re-import this cluster later: because the first six fields
above inherit, Slurm cannot distinguish "explicitly set to the same value as
the account" from "inheriting from the account" for `default_qos`,
`max_jobs`, `max_tres_per_job`, `max_tres_per_node`, and
`max_tres_mins_per_job` — the importer's response is to omit each field
whenever it matches the account's own value, on the assumption that a match
means inheritance.

The reference member above only sets one `max_tres_per_job` type (`gpu`)
because this illustrative account doesn't set that field at all — nothing to
merge with. A **real** account that also sets `max_tres_per_job` needs every
type it sets restated in the member's `account_overrides` too, or the two
won't converge — see **"The three TRES-list fields that inherit ... merge
per-TRES-type on read"** under [Notes](#notes) below, and
`data/accounts/lab_physics.yaml` / `lab_bio.yaml` for live-tested examples of
restating a shared type.

### What this layout cannot express

The two tables above are the *complete* set of fields these YAML files can
carry. If a field isn't in one of them, that's not a gap in this example —
it's not part of the schema, or it's deliberately kept out of `data/`.
Concretely, out of scope for `data/accounts/*.yaml` and `data/users.yaml`:

- **QOS definitions themselves** — `priority`, `max_wall_pj`, `flags`,
  `preempt_list`, `preempt_mode`, and a QOS's own TRES limits belong to a
  different resource, `slurm_qos`, kept as plain HCL in `qos.tf` (see the
  comment at the top of that file). QOS names and limits change rarely
  enough that inverting them through `data/` isn't worth it. `allowed_qos`
  and `default_qos` in the tables above only ever *reference* a QOS by
  name — they can't define one. See
  [`docs/resources/qos.md`](../../docs/resources/qos.md).
- **The `root` account** can never be an entry in `data/accounts/` — it's
  Slurm's built-in top-level account, not something `slurm_account` creates,
  updates, or destroys. Same reasoning as never managing the built-in
  `normal` QOS (see Bug 3 in `CLAUDE.md` and the QOS section of
  `tools/generate_import/README.md`).
- **Multi-cluster fan-out.** This provider is single-cluster-scoped — the
  cluster is set once, on the provider block in `main.tf`. There is no
  per-account or per-user `cluster:` key here or anywhere else in the
  provider.
- **An association's `account`.** In raw HCL, `docs/resources/user.md`'s
  [`association` block](../../docs/resources/user.md#nestedblock--association)
  requires `account` explicitly. Here it's implicit: which account an
  association belongs to is determined entirely by *which account file's
  `user_associations` list the entry lives in* — there is no `account:` key
  to write under `account_overrides` or `association`.
- **`default_wc_key`** lives only in `data/users.yaml`, once per user,
  cluster-wide — it is not part of `account_overrides` or `association`.
  The provider has no per-association WCKey to expose (Slurm's WCKey is a
  user-level default, not an association attribute), so there's nothing to
  put in a per-account file for it.
- **`id`** is computed on every resource (it mirrors `name`) and is never a
  config input, here or in any other layout.
- **`parent_account`** *is* in the account-level table above — it exists on
  `slurm_account` — but no real file under `data/accounts/` in this example
  actually sets it. That's intentional: see the `parent_account` note under
  [Notes](#notes) below for why it's unsafe in a self-contained,
  fully-Terraform-managed hierarchy like this one, and
  `examples/resources/slurm_account/resource.tf` for a live-tested
  parent/child example in the `flat` layout, where it's safe.

### Sanitized filenames

The YAML **filename stem** doesn't have to match the real Slurm account
name — add a `name:` key to override it. This matters for account names
containing characters that aren't safe in filenames (`/`, spaces, etc.):

```yaml
# data/accounts/external_collab.yaml
name: "external/collab"          # the real Slurm account name
description: "Cross-institution collaboration account"
fairshare: 20
user_associations:
  - alice
```

The importer (`--layout big-cluster`) does this automatically — it sanitizes
every account name into a safe filename stem and always writes the real name
into `name:`, so a hand-edited file only needs to add `name:` when you
create a new account whose name isn't already filename-safe.

## `data/users.yaml` — exceptions only

This file stays tiny. A user goes here **only** if they:

- belong to more than one account (pick their login `default_account`), or
- need an `admin_level`, or
- need a `default_wc_key`.

Single-account users with none of these are **not** listed — their default
account is derived from the one account file that lists them.

```yaml
john:
  default_account: lab_physics   # john is in 6 accounts; this is his login default
carol:
  admin_level: Administrator
  default_account: admin
# dave: not a real entry (he needs neither multi-account default nor
# admin_level, so he isn't listed at all in the actual data/users.yaml) --
# shown here only to illustrate default_wc_key syntax for a single-account
# user who did need one pinned:
dave:
  default_wc_key: genomics       # dave is single-account but needs a wckey pinned
```

> **`default_wc_key` caveat**: Slurm requires the WCKey to already be
> registered to the user (`sacctmgr add user <name> wckeys=<key>`, or
> self-service `sacctmgr add wckey <key>`) before it can become their
> default — otherwise Slurm silently ignores the change with no error.
> Separately, a known Slurm REST API limitation means `tofu plan` can never
> read this value back from Slurm to confirm it or detect drift; the write
> succeeds, but the field is effectively "write-only" from the provider's
> point of view.

## The worked multi-account user

`john` is a member of **6 accounts** (1 primary + 5 extra): `lab_physics`
(primary), `lab_bio`, `lab_chem`, `teaching`, `admin`, `shared`. He appears as
one entry in each of those 6 accounts' `user_associations`, plus one line in
`users.yaml` pinning his default. In `admin.yaml` his entry uses
`account_overrides` to get a different QOS there.

## Daily-ops cheat sheet

| Task | What to do |
|------|-----------|
| Who is in an account? | Open `data/accounts/<name>.yaml`, read `user_associations:` |
| Which accounts is a user in? | `grep -rl '<user>' data/accounts/` |
| Add a user to an account | Add `- <user>` under that account's `user_associations:` |
| Remove a user from an account | Delete their entry from that account's `user_associations:` |
| New single-account user | Add them to the one account file — nothing else |
| Make a user multi-account | Add them to extra account files + one line in `users.yaml` for their default |
| Give a user a per-account QOS | Use the object form, under `account_overrides`, in that account file |
| Give a user a per-account TRES/job-count/wall-clock limit | Use the object form; `account_overrides` for TRES/job/QOS fields, `association` for partition/priority/job-count/wall-clock fields — see "Supported per-user fields" above |
| Set an account-wide TRES/job limit | Add the field to that account's top-level YAML; see "Supported account-level fields" above |
| Scope a user's association to a partition | Use the object form's `association: { partition: ... }` — **not** on a user's own default account, see Notes |
| Grant admin rights | Add `admin_level` to their `users.yaml` entry |
| Pin a workload characterization key | Add `default_wc_key` to their `users.yaml` entry |

## Verifying without applying

Check the inversion is correct before touching the cluster:

```sh
tofu validate

# What accounts does each user resolve to?
echo '{ for u, v in local.users : u => sort([for a in v.associations : a.account]) }' | tofu console
```

## Debugging `generate.tf`

`generate.tf`'s `for_each`/`locals` inversion is expanded entirely **in
memory** during `tofu plan`/`apply` — there is no generated `.tf` file (or
any other file) written to disk that you can open and inspect. If the
resources OpenTofu plans don't match what you expect, work through the
inversion stage by stage:

1. **`tofu console` — inspect any local directly, cheapest option:**

   ```
   > local.accounts                              # raw YAML per account file
   > local.accounts.lab_physics.user_associations # one account's raw list
   > local.memberships                           # flattened, before inversion
   > local.users.john                            # john's final resolved shape
   > local.users.john.associations                # every association john gets
   ```

   This shows exactly what each stage of the inversion produces, before it
   ever reaches a resource — the fastest way to catch a YAML typo or a
   `try(...)` silently falling through to `null`.

2. **Isolate a single YAML file** if `yamldecode()` itself is erroring (bad
   indentation, wrong type, etc.) — an error against `local.accounts` points
   at the whole `for` expression, not the specific file. Decode one file
   directly instead:

   ```
   > yamldecode(file("data/accounts/lab_bio.yaml"))
   ```

3. **Plain `tofu plan`** already shows every `slurm_account.this["<key>"]` /
   `slurm_user.this["<key>"]` instance with all attributes fully resolved —
   no more `try()`/`for` expressions, just final values. Often this alone is
   enough.

4. **`tofu show -json`** for the fully resolved plan or state as structured
   JSON, useful for grepping/diffing many users at once:

   ```sh
   tofu plan -out=plan.tfplan
   tofu show -json plan.tfplan | jq '.resource_changes[] | select(.address == "slurm_user.this[\"john\"]")'

   # or, from state after apply:
   tofu show -json | jq '.values.root_module.resources[] | select(.address == "slurm_user.this[\"john\"]")'
   ```

5. **`tofu state show`** for one resource instance, human-readable:

   ```sh
   tofu state show 'slurm_user.this["john"]'
   ```

6. **`TF_LOG=debug tofu plan`** if the problem looks like a provider/API
   issue rather than a `generate.tf` logic issue — this logs the actual HTTP
   requests/responses to `slurmrestd`.

## Notes

- `generate.tf` adds `depends_on` so QOS are created before accounts, and
  accounts before user associations that reference them by name.
- Migrating an existing flat config to this layout changes resource addresses
  (`slurm_user.john` → `slurm_user.this["john"]`). Use `moved {}` blocks (or
  `tofu state mv`) so the switch is a no-op plan — these map to live Slurm
  entities and must not be destroyed/recreated.
- **Users with zero associations cannot be represented** in this
  account-centric layout — a user only exists here as an entry in some
  account's `user_associations` list. The importer (`--layout big-cluster`)
  skips such users and warns; single-user edits should keep this in mind.
  Users with no associations can't run jobs, so this is usually a non-issue.
  If you genuinely need to manage one, add it to `data/users.yaml` and
  extend `generate.tf` to create association-less `slurm_user` resources
  from those entries (not wired up by default).
- **`parent_account` is not ordering-safe across accounts in this layout.**
  All accounts share one `for_each`, so OpenTofu has no dependency
  information between a child account and its parent unless the parent is
  an already-existing, externally-managed account (a plain string, not a
  `slurm_account` resource in this configuration). Verified against a live
  cluster: creating a child alongside a not-yet-existing parent races, Slurm
  silently defaults the child's parent to `root` instead of erroring, and —
  separately — the provider's own `Read()` cannot detect this as drift
  (Slurm returns `parent_account: null` for an account under `root`, which
  looks identical to "no parent configured" and is never written back to
  state). A same-for_each self-reference to fix the ordering
  (`slurm_account.this[...]` from within `resource "slurm_account" "this"`)
  is not possible either — OpenTofu rejects it as a dependency cycle,
  regardless of whether the specific instances involved would actually
  cycle at runtime. **Until this is addressed, don't set `parent_account` to
  another account managed by this same `data/accounts/` directory.** A
  `parent_account` pointing at an account managed *outside* this
  configuration (created via `sacctmgr` or a separate Terraform root) is
  unaffected, since there's no shared `for_each` to race against. Multi-level
  hierarchies fully managed by Terraform work correctly today only in the
  `flat` layout, which computes real per-account `depends_on` from account
  depth.
- **Don't set `association.partition` for a user's own `default_account`.**
  Verified against a live cluster: `slurm_user`'s `Create()` bootstraps the
  user with an unscoped association for `default_account` before applying
  the real, plan-declared associations — if that same account's entry also
  sets `association.partition`, Slurm ends up with two associations
  (unscoped + partition-scoped) for the same user+account, and `tofu plan`
  shows a perpetual diff that never resolves. `partition` is safe on any
  *other* account a multi-account user belongs to. See
  `data/accounts/shared.yaml` (safe: john, non-default account) vs.
  `teaching.yaml` (dave's entry intentionally omits `partition` for this
  reason) and the matching entry in `CLAUDE.md`.
- **The three TRES-list fields that inherit (`max_tres_per_job`,
  `max_tres_per_node`, `max_tres_mins_per_job`) merge per-TRES-type on
  read, not as a whole list.** If an account sets `max_tres_per_job` for
  both `cpu` and `gres/gpu`, and a member's own
  `account_overrides.max_tres_per_job` only sets `gpu`, Slurm's association
  read returns *both* — the account's `cpu` entry plus the member's `gpu`
  entry — not just the explicit override. A config that only declares the
  `gpu` override will show perpetual drift trying to remove the `cpu` entry
  Slurm keeps re-adding. **Restate every TRES type the account sets on that
  field**, even the ones you're not changing — see
  `data/accounts/lab_physics.yaml` (john restates `cpu` alongside his own
  `gpu` override) and the matching entry in `CLAUDE.md`. This does not
  affect the importer (`--layout big-cluster` / `--layout flat`): it always
  captures the full, already-merged list Slurm returns, so a partial
  override is handled correctly by construction.
  `grp_tres`/`grp_tres_mins`/`grp_tres_run_mins` are **not** affected by
  this — per "What an omitted `account_overrides` key resolves to" above,
  they never inherit from the account at all (whole field or per-type), so
  there's nothing to merge and nothing to restate for them.
