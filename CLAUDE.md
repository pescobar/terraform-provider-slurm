# OpenTofu Slurm Provider - Project Context

## Overview

We are building an OpenTofu/Terraform provider in Go that manages Slurm accounting
resources via the slurmrestd REST API. The provider manages the same persistent
entities that `sacctmgr` handles — not ephemeral resources like jobs or reservations.

## Test Environment

- **Slurm version**: 25.05.4
- **API version**: v0.0.42
- **Cluster name**: linux
- **Endpoint**: http://localhost:6820
- **Auth**: JWT via `X-SLURM-USER-TOKEN` header (tokens expire every 30 min)
- **Docker**: Using `giovtorres/slurm-docker-cluster` with containers for mysql, slurmdbd, slurmctld, slurmrestd, and compute nodes
- **Port 6820** is exposed to the host

## Design Decisions (Final)

### Resources
1. **`slurm_cluster`** — name only. Cluster also configured at provider level.
2. **`slurm_account`** — name, description, organization, parent_account, plus limits as **direct attributes** (fairshare, default_qos, allowed_qos). Internally the provider makes two API calls: one for account metadata, one for the account-level association.
3. **`slurm_qos`** — name, description, priority, max_wall_pj, flags, preempt_list, preempt_mode.
4. **`slurm_user`** — name, admin_level, default_account, with **embedded association blocks** (TypeSet keyed by account+partition).

### Association Model
- Associations are **embedded in `slurm_user`** as `SetNestedBlock`, NOT separate resources.
- Each association block has: account, partition, fairshare, default_qos, max_jobs, qos.
- The Update function diffs old vs new associations and makes individual API calls.
- **Operation order for Update**: Create new associations → Update user defaults → Update changed associations → Delete removed associations.
- This ordering prevents edge cases when changing default_account.

### Provider Config
```hcl
provider "slurm" {
  endpoint    = "http://localhost:6820"
  token       = var.slurm_jwt_token
  cluster     = "linux"
  api_version = "v0.0.42"
}
```
- All config values can also come from env vars: SLURM_REST_URL, SLURM_JWT_TOKEN, SLURM_CLUSTER, SLURM_API_VERSION.
- Default api_version is v0.0.42.

### Other Decisions
- Single cluster scope only (no multi-cluster support).
- Slurm API uses POST for both create and update (upsert semantics).
- Provider uses terraform-plugin-framework (not SDKv2).
- Binary must be named `terraform-provider-slurm` (required by both Terraform and OpenTofu).
- For dev testing, use `~/.tofurc` with dev_overrides to skip registry.

## Slurm REST API Endpoints Used

### Accounts
| Operation | Method | Endpoint |
|-----------|--------|----------|
| Read one | GET | `/slurmdb/v0.0.42/account/{name}` |
| List all | GET | `/slurmdb/v0.0.42/accounts/` |
| Create/Update | POST | `/slurmdb/v0.0.42/accounts/` |
| Delete | DELETE | `/slurmdb/v0.0.42/account/{name}` |

### Users
| Operation | Method | Endpoint |
|-----------|--------|----------|
| Read one | GET | `/slurmdb/v0.0.42/user/{name}` |
| List all | GET | `/slurmdb/v0.0.42/users/` |
| Create with assoc | POST | `/slurmdb/v0.0.42/users_association/` |
| Update | POST | `/slurmdb/v0.0.42/users/` |
| Delete | DELETE | `/slurmdb/v0.0.42/user/{name}` |

### QOS
| Operation | Method | Endpoint |
|-----------|--------|----------|
| Read one | GET | `/slurmdb/v0.0.42/qos/{name}` |
| List all | GET | `/slurmdb/v0.0.42/qos/` |
| Create/Update | POST | `/slurmdb/v0.0.42/qos/` |
| Delete | DELETE | `/slurmdb/v0.0.42/qos/{name}` |

### Associations
| Operation | Method | Endpoint |
|-----------|--------|----------|
| Read one | GET | `/slurmdb/v0.0.42/association/` (query params) |
| List all | GET | `/slurmdb/v0.0.42/associations/` |
| Create/Update | POST | `/slurmdb/v0.0.42/associations/` |
| Delete one | DELETE | `/slurmdb/v0.0.42/association/` (query params) |

### Clusters
| Operation | Method | Endpoint |
|-----------|--------|----------|
| Read one | GET | `/slurmdb/v0.0.42/cluster/{name}` |
| Create/Update | POST | `/slurmdb/v0.0.42/clusters/` |
| Delete | DELETE | `/slurmdb/v0.0.42/cluster/{name}` |

## Bugs Found and Fixed

### Bug 1: Account association missing `user` field
- **Symptom**: `slurm API error (HTTP 500): Missing required field 'user' in dictionary`
- **Cause**: `Association.User` had `json:"user,omitempty"` which omitted the field for account-level associations
- **Fix**: Changed to `json:"user"` so empty string is sent

### Bug 2: QOS wall_clock type mismatch
- **Symptom**: `json: cannot unmarshal object into Go struct field QOSWallClockPer.qos.limits.max.wall_clock.per.job of type int`
- **Cause**: API returns `{"number": 1440, "set": true, "infinite": false}` but struct had `Job int`
- **Fix**: Changed `QOSWallClockPer.Job` from `int` to `*SlurmInt`

### Bug 3: "Slurmdbd query returned with empty list" on second apply
- **Symptom**: `slurm API error (HTTP 200): Slurmdbd query returned with empty list` on the second `tofu apply` after a destroy cycle
- **Cause**: The test config used `name = "normal"` which is Slurm's built-in system QOS (auto-created at DB init). On destroy the provider deletes it (soft-delete: `deleted=1`). On the next apply, slurmdbd does an UPDATE to restore the soft-deleted row; slurmrestd's internal verification query then fails to find the row because it uses conditions that don't match a restored system QOS.
- **Fix**: Never manage Slurm's built-in QOS names (`normal`, etc.) as provider resources. Rename all test/example QOS to non-system names (`standard`, `priority`, etc.).
- **Note**: `SlurmInt.Infinite` uses `omitempty` to avoid sending `"infinite":false` explicitly, which is a separate but related concern.

## Key Implementation Notes

### Null-Preservation Pattern (import behaviour)

`slurm_account` and `slurm_user` use **Optional-only** schema fields (no `Computed`).
The Read functions only update an Optional field in state when the prior state value is
already non-null. This prevents Slurm's inherited defaults (fairshare=1, inherited QOS,
etc.) from appearing as drift after a fresh import where the prior state is empty.

Consequence for import: after `tofu import`, all Optional fields start null. A
**reconcile apply** (`tofu apply`) is required to write config-declared values to
Slurm and populate state. After that, `tofu plan` must be clean.

The `qos` field in `slurm_user` association blocks has a stricter rule: it is
**never populated from Slurm during import** (`hasPrior && !prior.QOS.IsNull()`
guard in `apiAssociationsToState`). Reason: if the existing qos list were loaded
into state but config has no `qos` block, the reconcile apply would try to clear
the list. Slurm rejects that when `default_qos` references a QOS in that list
(`"This request would make it so some associations would not have access to their
default qos."`).

`slurm_qos` uses Optional+Computed fields and does NOT have this restriction —
all QOS attributes are read from Slurm during import, and no reconcile apply is
needed.

### Association Diff Logic (`user_association_diff.go`)
- Pure functions, no side effects, independently testable.
- `DiffAssociations(old, new) AssociationDiff` returns Create/Update/Delete lists.
- Keys by `account + partition`.
- Deep comparison: fairshare, default QOS, QOS list (order-insensitive), max jobs.
- Deterministic sorted output.
- **14 unit tests** covering: no changes, QOS order irrelevance, create-only, delete-only, update-only, mixed operations, partition-scoped, nil↔value transitions, single↔multi account, default account changes, complete replacement.

### Critical Test Reminder
The association diff/update logic is the most bug-prone part. Unit tests are in `user_association_diff_test.go`. Run with:
```bash
go test ./internal/resources/ -v -run TestDiff
```

## Project Structure
```
terraform-provider-slurm/
├── main.go
├── go.mod                    # also pins tfplugindocs (via tools/tools.go)
├── Makefile
├── .golangci.yml             # minimal lint config (v2 standard set)
├── internal/
│   ├── provider/
│   │   └── provider.go
│   ├── client/               # split by entity: client.go, retry.go,
│   │   └── …                 #   cluster.go, account.go, qos.go, user.go, assoc.go
│   └── resources/
│       ├── account.go / qos.go / user.go (+ *_data_source.go)
│       ├── helpers.go        # shared conversion + Configure helpers
│       ├── user_association_diff.go
│       └── *_test.go
├── examples/                 # END-USER facing only
│   ├── main.tf               #   basic working example
│   ├── big-cluster/          #   data-driven layout for large clusters
│   │   ├── generate.tf       #     locals + for_each (inverts data → resources)
│   │   ├── qos.tf
│   │   └── data/             #     account-centric YAML the sysadmins edit
│   │       ├── accounts/*.yaml
│   │       └── users.yaml
│   ├── provider/             #   registry-doc fragments (tfplugindocs reads
│   ├── resources/            #   these paths — do not move them)
│   └── data-sources/
├── test/
│   ├── setup_test_data.sh
│   └── fixtures/             # acceptance-test configs driven by CI
│       ├── advanced-acceptance-tests/, assoc-limits-tests/,
│       ├── data-source-tests/, qos-acceptance-tests/,
│       ├── system-qos-warning/, user-association-tests/,
│       └── validator-tests/
└── tools/
    ├── tools.go              # keeps tfplugindocs pinned in go.mod
    └── generate_import/      # HCL/YAML generator for existing clusters
```

### Client conventions

- **Context propagation**: every `client.Client` method takes `ctx` as its
  first parameter; requests use `http.NewRequestWithContext` and the retry
  backoff sleep is ctx-aware (`sleepCtx`). Cancelling the Terraform operation
  aborts in-flight calls and pending retries.
- **User-Agent**: every request sends `terraform-provider-slurm/<version>`
  (set in provider Configure) for slurmrestd log attribution.
- **Delete is idempotent**: slurmrestd returns HTTP 304 for DELETE on a
  nonexistent account/user/QOS/association (verified against 25.05.4), and
  `doRequestOnce` treats 304 as success — so deleting an out-of-band-removed
  resource does not fail. Locked in by `TestDoRequest_TreatsNotModifiedAsSuccess`.

### Large-cluster layout (`examples/big-cluster/`)

For clusters with hundreds of accounts and thousands of users, a flat
`users.tf` is unmaintainable. The `big-cluster` example demonstrates a
data-driven alternative: sysadmins edit **account-centric** YAML
(`data/accounts/<name>.yaml` holds account metadata + a member list;
`data/users.yaml` holds only exceptions — admins and multi-account default
picks), and `generate.tf` **inverts** that into the **user-centric**
`slurm_user`/`slurm_account` resources the provider needs (a `slurm_user`
carries all of its associations, so a multi-account user cannot be split across
per-account files in raw HCL — hence the inversion). Membership is normalized
with `try(m.user, m)` to accept both bare-string and object-with-overrides
members; a `?:` conditional is avoided because Terraform won't unify `string`
with `null`. Migrating a flat config to this layout changes resource addresses,
so `moved {}` blocks (or `tofu state mv`) are required to avoid
destroy/recreate of live Slurm entities.

To import an existing cluster directly into this layout, run
`tools/generate_import/generate_import.py --layout big-cluster` — it emits the
same self-contained structure (data YAML + `generate.tf` + `for_each`-addressed
`imports.tf`). The flat and big-cluster emitters share all field-extraction
logic (`_account_fields` / `_assoc_fields`) so the two layouts cannot drift.
The committed `examples/big-cluster/generate.tf` is produced from the same
template as the generator's output.

## Current Status
- All four resources implemented (cluster, account, qos, user with embedded associations).
- Three bugs found and fixed (see above).
- Integration tested: apply/destroy/apply cycle works reliably with non-system QOS names.
- CI: unit tests, golangci-lint, docs-check, and an acceptance matrix across
  three Slurm versions (25.05 / 25.11 / 26.05) driving the configs under
  `test/fixtures/`; releases via goreleaser on tags.

## What's Left
- ~~Import support needs testing~~ — done (generate_import workflow + reconcile-apply pattern).
- ~~Acceptance tests~~ — done, example-dir style under `test/fixtures/` run by CI (not terraform-plugin-testing).
- ~~CI with GitHub Actions~~ — done (unit-tests, lint, docs-check, acceptance matrix, release).
- Auth improvements (JWT key file instead of short-lived tokens)
- ~~Handle the `normal` QOS corner case~~ — done. `slurm_qos` `ValidateConfig` now emits a Terraform warning when a managed QOS name matches an entry in `systemQOSNames` (currently just `normal`). The list is package-level so additional system QOS names can be added without changing the validator. Covered by `TestSystemQOSWarning_*` unit tests and the `test/fixtures/system-qos-warning/` plan-only acceptance fixture.
