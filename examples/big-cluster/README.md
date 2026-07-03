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
    qos: [debug, standard]
```

The account **file name** (minus `.yaml`) is the Slurm account name.

## `data/users.yaml` — exceptions only

This file stays tiny. A user goes here **only** if they:

- belong to more than one account (pick their login `default_account`), or
- need an `admin_level`.

Single-account users are **not** listed — their default account is derived from
the one account file that lists them.

```yaml
john:
  default_account: lab_physics   # john is in 6 accounts; this is his login default
carol:
  admin_level: Administrator
  default_account: admin
```

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
| Grant admin rights | Add `admin_level` to their `users.yaml` entry |

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
