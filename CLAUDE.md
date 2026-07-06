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
- Each association block has: account, partition, fairshare, default_qos, max_jobs, allowed_qos.
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

### Version-gated features (Slurm 26.05 / API v0.0.45)

`data.slurm_conf` and `data.slurm_dbd_conf` read the active slurmctld/slurmdbd
configuration via the `/conf` endpoints that only exist in API v0.0.45+
(Slurm 26.05). Two guard layers (`internal/client/conf.go`):

1. `requireAPIVersion` — deterministic pre-flight check against the configured
   `api_version`; fails `tofu plan` with an actionable message (names the
   feature, required Slurm release, configured version, and the fix) before
   any HTTP call. Unparsable api_versions pass through — the server stays
   authoritative.
2. `client.IsNotFound` 404 fallback — catches "api_version is new enough but
   the cluster runs older Slurm"; the diagnostic then points at the server.

CI covers both paths: the conf fixture runs only on the v0.0.45 matrix entry,
and a negative step asserts the version-error text on older entries. When
adding new version-gated endpoints, follow this same two-layer pattern.

**No `slurm_partition` resource, deliberately**: Slurm 26.05 added partition
create/update/delete via REST, but REST-created partitions are not written to
slurm.conf and vanish on slurmctld restart (verified empirically on 26.05.1) —
they would drift on every controller restart. Partitions are exposed as the
read-only `data.slurm_partition` (works on all supported versions, v0.0.42+).
Revisit if SchedMD makes REST-created partitions persistent.

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

### Known limitation: `default_wc_key` cannot be read back from the REST API
- **Symptom**: setting `slurm_user.default_wc_key` applies successfully (verified: `sacctmgr show user` confirms the value is stored server-side), but `GET /user/{name}`, `GET /users/?with_assocs=true`, and `GET /associations/` all return `default.wckey` as an empty string — regardless of API version (checked v0.0.42–v0.0.45) — and `GET /wckeys/` returns the WCKey's existence but nothing marking it as anyone's *default* (its `flags` enum only contains `DELETED`).
- **Cause**: an upstream Slurm REST API gap, not something fixable in this provider.
- **Consequence**: `default_wc_key` behaves correctly on write but `Read()` can never confirm or detect drift on it — the null-preservation guard (`user.Default.WCKey != "" && !state.DefaultWCKey.IsNull()`) simply never fires, since the API-returned value is always `""`. `tools/generate_import/generate_import.py` has the matching limitation: it reads from the same field and so will never emit `default_wc_key` for any user, even when one is set. See `tools/generate_import/README.md`'s Users section for the user-facing note.
- **Prerequisite discovered along the way**: Slurm also requires a WCKey to already be registered against the user (`sacctmgr add user <name> wckeys=<key>`, or self-service `sacctmgr add wckey <key>`) before it can become that user's default — setting an unregistered WCKey as default silently no-ops (no error from the REST API; `sacctmgr` itself reports "aren't associated with new default wckey").

### Known limitation: `parent_account` drift is invisible, and multi-level hierarchies race in `examples/big-cluster/`
- **Symptom (drift-blindness, core provider)**: for an account under `root` (no parent configured, or a parent that silently failed to apply — see next point), `GET /account/{name}` returns `parent_account: null`. `slurm_account`'s `Read()` only writes `parent_account` into state when the API value is non-empty (`account.ParentAccount != "" && !state.Parent.IsNull()`), so a `null`/empty API response is indistinguishable from "field not tracked" — an out-of-band change that moves an account to `root` (or any other silent failure to apply the configured parent) is never surfaced as drift by `tofu plan`.
- **Symptom (race, `examples/big-cluster/` specifically)**: `generate.tf`'s `resource "slurm_account" "this"` shares one `for_each` across every account; OpenTofu has no ordering guarantee between a child and its not-yet-existing parent unless there's an explicit dependency edge. Verified against a live cluster: applying a parent/child pair defined this way can create the child first, and Slurm silently defaults `parent_account` to `root` rather than erroring — combined with the drift-blindness above, `tofu plan` shows this as a clean, matching state forever after.
- **Attempted fix, rejected**: making `parent_account` reference `slurm_account.this[...]` from within `resource "slurm_account" "this"` itself (to get automatic dependency ordering) fails with `Error: Cycle: slurm_account.this[...], slurm_account.this[...]` — OpenTofu rejects a resource's config referencing other instances of the *same* resource via a dynamic (data-dependent) index, even when the specific instances involved would never actually cycle at runtime. This appears to be a hard limitation of OpenTofu/Terraform's dependency graph, not something the provider or this config can route around locally.
- **Current guidance**: don't set `parent_account` to another account managed by the same `data/accounts/` directory in the big-cluster layout. The `flat` layout is unaffected — `generate_accounts()` computes real account depth and per-account `depends_on` since each account gets its own resource label rather than sharing a `for_each`. See `examples/big-cluster/README.md`'s Notes section.
- The drift-blindness half of this (independent of the big-cluster race) is now also documented in the published registry docs — see the "`parent_account` drift is not detected once set" section in `templates/resources/account.md.tmpl` / `docs/resources/account.md` — since it affects `slurm_account` generally, not just that example.

### Known limitation: a `partition`-scoped association on a user's default account creates a phantom duplicate
- **Symptom**: `slurm_user.Create()` first calls the atomic `users_association/` endpoint to bootstrap the user with an association for `default_account` (no `partition` in that call — see `client.UserAssociationCondition`), then separately calls `CreateAssociations()` with the full, plan-declared association list. If the config's association *for that same default account* also sets `partition`, Slurm ends up with **two** associations for that user+account: the unscoped one from the bootstrap call, and the partition-scoped one from the real config. `Read()` then reports both, and `tofu plan` shows a perpetual diff trying to remove the phantom unscoped one — which reappears every apply, since the bootstrap step recreates it.
- **Verified against a live cluster**: reproduced with a single-account user whose one association set `partition`; confirmed via `GET /associations/?user=<name>` returning two rows (`partition: ""` and `partition: "<value>"`) for the same account.
- **Workaround**: don't set `partition` on the association for a user's own `default_account`. It's safe on any *other* account the user belongs to (multi-account users only), since only the bootstrap call for `default_account` has this behavior. `examples/big-cluster/data/accounts/shared.yaml` demonstrates the safe case (`association: { partition: cpu }` on john's non-default account); `teaching.yaml` has a comment explaining why dave's (default-account) entry doesn't use it.
- **Not yet fixed**: a real fix would need `Create()` to either pass `partition` through the bootstrap call when the default-account association has one, or skip/delete the bootstrap association when a same-account, partition-scoped one is about to be created in the same apply.

### Known limitation: TRES limits merge per-type on read, so a partial override never converges
- **Symptom**: when an account sets `max_tres_per_job` (or `max_tres_per_node` / `max_tres_mins_per_job` — the three TRES-list fields that inherit the account's value when unset) for types `{cpu, gres/gpu}` and a user's own association overrides only one of those types (e.g. just `gres/gpu`), Slurm's association read returns the **merged** set — the account's `cpu` entry plus the user's own `gpu` entry — not just the user's explicit override. If the HCL/YAML config declares only the user's intended override (`gpu` alone), `tofu plan` shows perpetual drift trying to remove the `cpu` entry Slurm keeps re-adding. `grp_tres`/`grp_tres_mins`/`grp_tres_run_mins` do **not** inherit the account's value at all (whole field or per-type) — see `_assoc_fields()`'s docstring in `tools/generate_import/generate_import.py` — so they're unaffected by this and never need restating.
- **Verified against a live cluster**: `GET /associations/?user=<name>&account=<account>` for an association whose config set only `{gres/gpu: 2}` returned `[{cpu: 64}, {gres/gpu: 2}]` — the `cpu: 64` came from the account's own `max_tres_per_job`, not the user's config.
- **Workaround**: when partially overriding one of the three inheriting TRES-list fields at the association level, restate every TRES type the parent account sets on that same field, not just the type(s) you're actually changing. `examples/big-cluster/data/accounts/lab_physics.yaml` demonstrates this (john's `account_overrides.max_tres_per_job` restates `cpu: 64` alongside his own `gres/gpu: 2`).
- **Note for `tools/generate_import/generate_import.py`**: the importer's inheritance-detection fix (see the `_assoc_fields()` changelog entry) already handles this correctly by construction — it captures whatever *raw* (already-merged) TRES list Slurm returns and only skips emitting it when that whole list matches the account's own, so a partial override like this one is captured in full automatically. This limitation only bites **hand-written** configs that don't know to restate the untouched types.

### Known limitation: `slurm_user.Create()` can leave an orphaned, untracked user in Slurm on a partial failure
- **Symptom**: `Create()` bootstraps the user via an atomic `users_association/` call (Step 1, no QOS/limits), then calls `CreateAssociations()` with the full plan-declared association(s) (Step 2). If Step 1 succeeds but Step 2 fails (e.g. a QOS access-constraint violation — the same class of error `qosAccessHint` explains), the framework never calls `resp.State.Set()` before returning the error, so Terraform records **no state at all** for that resource — even though the user now genuinely exists in Slurm. `tofu destroy` can't clean up a resource it never tracked.
- **Verified against a live cluster**: reproduced via `test/fixtures/user-association-tests/negative/` (intentionally violates the QOS-access rule). After the expected-to-fail `tofu apply`, `tofu state list` showed only the QOS/account resources — not the two `slurm_user` resources — while `sacctmgr show user` confirmed both users existed server-side. Deleting the *account* those orphaned users pointed at (via that fixture's own, in-state `tofu destroy`) cascade-deletes their associations too, leaving a genuinely **zero-association** user behind — which is exactly the input that exposed the `generate_import.py` bug below.
- **Consequence for `tools/generate_import/generate_import.py`**: `generate_users()` (the `flat` layout) used to assume every user has at least one association and unconditionally emitted a `slurm_user` block, which fails Terraform validation for a zero-association user (`default_account` is `Required`, and at least one `association` block is required). Fixed to skip such users with a warning, mirroring `generate_bigcluster_users()`'s existing behavior for the big-cluster layout.
- **CI mitigation, not a provider fix**: the negative-test fixture's CI step now deletes `neg_u_rule1`/`neg_u_rule2` directly via the REST API after its own `tofu destroy`, so they don't leak into later steps in the same job (in particular the `generate_import.py` acceptance step, which enumerates every user on the cluster).
- **Not yet fixed at the provider level**: a real fix would need `Create()` to either write partial state after Step 1 succeeds (so `tofu destroy` can clean it up even when Step 2 fails), or attempt to roll back (delete) the Step-1-created user when Step 2 fails. Both are reasonable but nontrivial design choices, deliberately not made here.

## Key Implementation Notes

### Null-Preservation Pattern (import behaviour)

`slurm_account` and `slurm_user` use **Optional-only** schema fields (no `Computed`).
The Read functions only update an Optional field in state when the prior state value is
already non-null. This prevents Slurm's inherited defaults (fairshare=1, inherited QOS,
etc.) from appearing as drift after a fresh import where the prior state is empty.

Consequence for import: after `tofu import`, all Optional fields start null. A
**reconcile apply** (`tofu apply`) is required to write config-declared values to
Slurm and populate state. After that, `tofu plan` must be clean.

The `allowed_qos` field in `slurm_user` association blocks (named `qos` before
v2.0.0 — renamed for consistency with `slurm_account.allowed_qos`, the same
Slurm association-QOS concept at a different scope) has a stricter rule: it is
**never populated from Slurm during import** (`hasPrior && !prior.AllowedQOS.IsNull()`
guard in `apiAssociationsToState`). Reason: if the existing QOS list were loaded
into state but config has no `allowed_qos` set, the reconcile apply would try to
clear the list. Slurm rejects that when `default_qos` references a QOS in that
list (`"This request would make it so some associations would not have access to
their default qos."`).

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
(`data/accounts/<name>.yaml` holds account metadata + a `user_associations`
list; `data/users.yaml` holds only exceptions — admins and multi-account
default picks), and `generate.tf` **inverts** that into the **user-centric**
`slurm_user`/`slurm_account` resources the provider needs (a `slurm_user`
carries all of its associations, so a multi-account user cannot be split across
per-account files in raw HCL — hence the inversion).

Each `user_associations` entry is normalized with `try(m.user, m)` to accept
both a bare string ("alice", no overrides) and an object form. The object
form splits association attributes into two sub-maps so the "override
account defaults" vs "declare association-only data" distinction is visible
in the YAML itself, not just in prose (see "Two kinds of per-user data" in
`examples/big-cluster/README.md`):
- `account_overrides` — fields `slurm_account` also has (fairshare,
  default_qos, allowed_qos, max_jobs, TRES limits); a value here overrides
  what the member would otherwise inherit from the account.
- `association` — fields that exist only at the association level
  (partition, priority, job-count/wall-clock limits); there's nothing to
  inherit, these are declared, not overridden.

`try(m.account_overrides.fairshare, null)`-style lookups read each field, so
a bare-string entry (which has neither sub-map) safely falls through to
`null` for every field. A `?:` conditional is avoided because Terraform
won't unify `string` with `null`. Migrating a flat config to this layout
changes resource addresses, so `moved {}` blocks (or `tofu state mv`) are
required to avoid destroy/recreate of live Slurm entities.

To import an existing cluster directly into this layout, run
`tools/generate_import/generate_import.py --layout big-cluster` — it emits the
same self-contained structure (data YAML + `generate.tf` + `for_each`-addressed
`imports.tf`). The flat and big-cluster emitters share all field-extraction
logic (`_account_fields` / `_assoc_fields`) so the two layouts cannot drift.
The committed `examples/big-cluster/generate.tf` is produced from the same
template as the generator's output.

## Current Status
- All four resources implemented (cluster, account, qos, user with embedded associations).
- Data sources for every managed entity (`slurm_qos`, `slurm_account`, `slurm_user`),
  plus read-only `slurm_partition`, and version-gated `slurm_conf`/`slurm_dbd_conf`
  (Slurm 26.05+ / API v0.0.45+).
- Three bugs found and fixed (see above).
- Integration tested: apply/destroy/apply cycle works reliably with non-system QOS names.
- `insecure_skip_ssl_verify` provider option for self-signed-certificate HTTPS endpoints
  (secure-by-default; opt-in only, with a plan-time warning).
- CI: unit tests, golangci-lint, docs-check, and an acceptance matrix across
  three Slurm versions (25.05 / 25.11 / 26.05) driving the configs under
  `test/fixtures/`; releases via goreleaser on tags.

## What's Left
- ~~Import support needs testing~~ — done (generate_import workflow + reconcile-apply pattern).
- ~~Acceptance tests~~ — done, example-dir style under `test/fixtures/` run by CI (not terraform-plugin-testing).
- ~~CI with GitHub Actions~~ — done (unit-tests, lint, docs-check, acceptance matrix, release).
- Auth improvements (JWT key file instead of short-lived tokens)
- ~~Handle the `normal` QOS corner case~~ — done. `slurm_qos` `ValidateConfig` now emits a Terraform warning when a managed QOS name matches an entry in `systemQOSNames` (currently just `normal`). The list is package-level so additional system QOS names can be added without changing the validator. Covered by `TestSystemQOSWarning_*` unit tests and the `test/fixtures/system-qos-warning/` plan-only acceptance fixture.
