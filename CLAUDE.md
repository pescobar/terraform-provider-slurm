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
├── go.mod
├── Makefile
├── internal/
│   ├── provider/
│   │   └── provider.go
│   ├── client/
│   │   └── client.go
│   └── resources/
│       ├── cluster.go
│       ├── account.go
│       ├── qos.go
│       ├── user.go
│       ├── user_association_diff.go
│       └── user_association_diff_test.go
├── examples/
│   └── main.tf
└── test/
    └── setup_test_data.sh
```

## Current Status
- All four resources implemented (cluster, account, qos, user with embedded associations).
- Three bugs found and fixed (see above).
- Integration tested: apply/destroy/apply cycle works reliably with non-system QOS names.

## What's Left
- Import support needs testing
- Acceptance tests
- CI with GitHub Actions
- Auth improvements (JWT key file instead of short-lived tokens)
- Handle the `normal` QOS corner case: Slurm auto-creates a QOS named `normal` at DB init; managing it as a `slurm_qos` resource will fail on destroy+recreate (see Bug 3). At minimum this needs a documentation warning; optionally the provider could emit a Terraform warning diagnostic when it detects a system QOS name.
