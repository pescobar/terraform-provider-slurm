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
    │   ├── lab_physics.yaml   # one file per account: metadata + members
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
members:
  - alice                 # simple: bare username
  - user: john            # object form, only when this membership needs overrides
    default_qos: debug
    allowed_qos: [debug, standard]
```

The account **file name** (minus `.yaml`) is the Slurm account name.

### Supported account-level fields

`generate.tf` passes every one of these through to `slurm_account` if present
in the account YAML; all are optional besides the members it inverts:

| Key | Maps to `slurm_account` attribute |
|-----|-----------------------------------|
| `description`, `organization`, `parent_account` | same-named attributes |
| `fairshare`, `default_qos`, `allowed_qos`, `max_jobs` | same-named attributes |
| `max_tres_per_job`, `max_tres_per_node`, `max_tres_mins_per_job` | per-job TRES limits |
| `grp_tres`, `grp_tres_mins`, `grp_tres_run_mins` | group-aggregate TRES limits |

TRES fields use the same list-of-objects shape as the `slurm_account`
resource (`type`, optional `name` for generic resources like `gres`, `count`):

```yaml
description: "Physics Lab"
fairshare: 100
max_jobs: 200
max_tres_per_job:
  - type: cpu
    count: 64
  - type: gres
    name: gpu
    count: 8
grp_tres_mins:
  - type: cpu
    count: 500000
members:
  - alice
```

### Supported per-member override fields (object form)

The object form of a member (`- user: <name>` plus override keys) accepts
every field `slurm_user`'s `association` block accepts — not just QOS.
Everything from `partition` and `max_jobs` down to per-member TRES limits can
be overridden for a single user in a single account:

```yaml
members:
  - alice
  - user: bob                 # bob gets a tighter per-account TRES cap
    max_jobs: 5
    max_tres_per_job:
      - type: gres
        name: gpu
        count: 2
```

Any key omitted from a member override falls back to Slurm's own default for
that association (not the account's value) — set it explicitly on the member
if you want it to match the account.

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
(primary), `lab_bio`, `lab_chem`, `teaching`, `admin`, `shared`. He appears as a
one-line member in each of those 6 account files, plus one line in `users.yaml`
pinning his default. In `admin.yaml` he uses the object form to get a different
QOS there.

## Daily-ops cheat sheet

| Task | What to do |
|------|-----------|
| Who is in an account? | Open `data/accounts/<name>.yaml`, read `members:` |
| Which accounts is a user in? | `grep -rl '<user>' data/accounts/` |
| Add a user to an account | Add `- <user>` under that account's `members:` |
| Remove a user from an account | Delete their line from that account's `members:` |
| New single-account user | Add them to the one account file — nothing else |
| Make a user multi-account | Add them to extra account files + one line in `users.yaml` for their default |
| Give a user a per-account QOS | Use the object form in that account file |
| Give a user a per-account TRES/job limit | Use the object form; see "Supported per-member override fields" above |
| Set an account-wide TRES/job limit | Add the field to that account's YAML; see "Supported account-level fields" above |
| Grant admin rights | Add `admin_level` to their `users.yaml` entry |
| Pin a workload characterization key | Add `default_wc_key` to their `users.yaml` entry |

## Verifying without applying

Check the inversion is correct before touching the cluster:

```sh
tofu validate

# What accounts does each user resolve to?
echo '{ for u, v in local.users : u => sort([for a in v.associations : a.account]) }' | tofu console
```

## Notes

- `generate.tf` adds `depends_on` so QOS are created before accounts, and
  accounts before user associations that reference them by name.
- Migrating an existing flat config to this layout changes resource addresses
  (`slurm_user.john` → `slurm_user.this["john"]`). Use `moved {}` blocks (or
  `tofu state mv`) so the switch is a no-op plan — these map to live Slurm
  entities and must not be destroyed/recreated.
- **Users with zero associations cannot be represented** in this
  account-centric layout — a user only exists here as a member of an account.
  The importer (`--layout big-cluster`) skips such users and warns; single-user
  edits should keep this in mind. Users with no associations can't run jobs, so
  this is usually a non-issue. If you genuinely need to manage one, add it to
  `data/users.yaml` and extend `generate.tf` to create association-less
  `slurm_user` resources from those entries (not wired up by default).
