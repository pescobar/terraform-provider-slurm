# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.2.1] - 2026-07-04

### Breaking Changes

- **`slurm_user` association attribute `qos` renamed to `allowed_qos`.** It
  was the same Slurm concept as `slurm_account.allowed_qos` (an
  association's QOS list) but named differently depending on which resource
  exposed it — confusing enough in practice to fix now, while the schema is
  still young (pre-1.0; breaking changes ship in minor/patch releases per
  semver's rules for `0.x` versions).

  **Migration**: rename `qos = [...]` to `allowed_qos = [...]` inside every
  `slurm_user` `association` block (`slurm_account.allowed_qos` is
  unaffected — it was already named `allowed_qos`). No resource replacement
  occurs and no manual state surgery is needed: after updating your `.tf`
  files, `tofu plan`/`apply` reconciles the association in place using the
  normal update path (same account+partition key). The `qosAccessHint`
  diagnostic text and all examples, fixtures, and registry docs have been
  updated to match.

### Added

- **`examples/big-cluster/` README now documents every supported field**
  with complete reference tables and full worked YAML examples for both
  account-level fields (all 13) and per-member override fields (all 19),
  plus a `name:` sanitized-filename example. The real `data/accounts/*.yaml`
  fixtures were enriched to exercise every field live (previously only a
  handful were demonstrated); verified end-to-end against a live 26.05.1
  cluster (`tofu apply` → clean `tofu plan` → `tofu destroy`).
- **Two previously-unknown Slurm/provider interactions discovered and
  documented** while verifying the above (both confirmed against a live
  cluster; not fixed in this release — see `CLAUDE.md`'s "Known
  limitation" entries and the corresponding Notes in
  `examples/big-cluster/README.md`):
  - Setting `partition` on the association for a user's own
    `default_account` creates a phantom duplicate (unscoped) association,
    because `slurm_user.Create()`'s atomic bootstrap call always creates an
    unscoped association for `default_account` before the real,
    plan-declared associations are applied. Safe on any other account a
    multi-account user belongs to.
  - TRES-list fields (`max_tres_per_job`, etc.) merge **per TRES type** on
    read: a member overriding only one TRES type (e.g. `gpu`) while the
    account sets others (e.g. `cpu`) will see Slurm return both merged
    together, so a config declaring only the intended override never
    converges — every type the account sets must be restated. The importer
    already handles this correctly by construction; it only affects
    hand-written configs.
  - Also confirmed (and reverted after testing): a `for_each`-shared
    `slurm_account` resource cannot reference other instances of itself to
    get automatic dependency ordering for `parent_account` — OpenTofu
    rejects it as a cycle even when the specific instances involved
    wouldn't actually cycle at runtime. Documented as a hard limitation
    alongside the pre-existing `parent_account` drift-blindness note.
- **`insecure_skip_verify` provider option** for connecting to slurmrestd
  over HTTPS with a self-signed or internally-issued certificate. TLS
  certificates are validated by default (secure by default); set
  `insecure_skip_verify = true` or `SLURM_INSECURE_SKIP_VERIFY=true` to skip
  validation. The provider emits a plan-time warning whenever it's enabled,
  and the setting has no effect over plain `http://`. Verified against a
  real self-signed-certificate HTTPS endpoint (both the default-secure
  rejection and the opt-in bypass).
- **Data sources for every managed entity**: `slurm_qos`, `slurm_account`,
  and `slurm_user` read existing Slurm entities by name without bringing
  them under provider management.
- **`slurm_partition` data source**: looks up a partition from slurmctld
  (state, flags, allow/deny accounts and QOS, limits, priority). Works with
  every supported Slurm release (API v0.0.42+). Partitions are deliberately
  exposed read-only: Slurm 26.05 added partition create/update/delete via
  REST, but REST-created partitions are not persisted to `slurm.conf` and
  vanish on slurmctld restart, so a managed resource would drift on every
  controller restart.
- **`slurm_conf` and `slurm_dbd_conf` data sources** (Slurm 26.05+ /
  API v0.0.45+): the active slurmctld / slurmdbd configuration as a
  `map(string)` plus typed convenience fields (`slurm_version`,
  `cluster_name`, `conf_path`). Useful for preflight assertions (e.g.
  `AccountingStorageEnforce` includes `associations`) and cluster facts.
- **Version-aware error handling**: features that need a newer Slurm release
  fail at plan time with an actionable message naming the feature, the
  required Slurm release and API version, the configured `api_version`, and
  the fix — before any HTTP call. An HTTP 404 fallback covers the inverse
  case (new `api_version`, older cluster). The provider's connectivity ping
  now also explains a 404 as an `api_version`/server mismatch and prints the
  version table.
- **Plan-time validation**: enum and range checks on resource attributes,
  cross-field invariants on `slurm_user` (`default_account` must match an
  association), and a warning when managing Slurm's built-in system QOS
  (`normal`).
- **Transient-failure retries**: the client retries retryable HTTP failures
  (5xx subset, 408, 429, network errors) with exponential backoff;
  deterministic Slurm rejections are returned immediately.
- Docker test cluster image for **Slurm 26.05.1**; the acceptance-test matrix
  now covers 25.05.4 (v0.0.42), 25.11.5 (v0.0.44), and 26.05.1 (v0.0.45).
  The docker README documents the Slurm-release → API-version mapping.
- Registry documentation: a Slurm version-compatibility matrix on the
  provider index page, per-page version-requirement callouts on the
  v0.0.45-only data sources, and CI validation of every registry doc snippet
  against the live provider schema.
- The API client now propagates `context.Context` through every request and
  retry backoff sleep: cancelling an operation (Ctrl-C, timeouts) aborts
  in-flight HTTP calls and pending retries immediately instead of letting
  them run to completion. Covered by new unit tests
  (`TestDoRequest_CancelledContextStopsRetries`, `TestSleepCtx_ReturnsEarlyOnCancel`).
- Every API request sends `User-Agent: terraform-provider-slurm/<version>`
  so slurmrestd logs can attribute provider traffic. Covered by
  `TestDoRequest_SendsAuthAndUserAgentHeaders`.
- CI: `golangci-lint` job (minimal v2 config: standard set + misspell,
  unconvert) and a `docs-check` job that fails when `docs/` drifts from the
  provider schema.
- `tools/generate_import/generate_import.py` gained a `--layout` flag. The new
  `--layout big-cluster` emits a complete, self-contained account-centric
  directory (`main.tf`, `qos.tf`, `generate.tf`, `data/accounts/*.yaml`,
  `data/users.yaml`, and `for_each`-addressed `imports.tf`) instead of the flat
  per-resource HCL. Account membership is inverted from Slurm's associations,
  `data/users.yaml` holds only exceptions (admins + multi-account defaults),
  account filenames are sanitized while the real name is preserved in a `name:`
  key, and association-less users are skipped with a warning. The flat and
  big-cluster emitters share all field-extraction logic so they cannot drift.
- `examples/big-cluster/`: a data-driven, human-friendly layout for managing
  large clusters (hundreds of accounts, thousands of users). Sysadmins edit
  account-centric YAML under `data/` (one file per account with its member
  list, plus a small `users.yaml` for exceptions), and `generate.tf` inverts
  it into the user-centric `slurm_user`/`slurm_account` resources via
  `for_each`. Includes a README with a daily-ops cheat sheet and a worked
  multi-account user example.
- Documented account-level and per-member **TRES limits** in the
  `examples/big-cluster/` README: `generate.tf` already wired
  `max_tres_per_job`/`max_tres_per_node`/`max_tres_mins_per_job`/`grp_tres`/
  `grp_tres_mins`/`grp_tres_run_mins` through for both account YAML files
  and per-member object-form overrides, but neither was documented. Added a
  reference table for both scopes and a worked example (verified against a
  live plan) to `lab_physics.yaml`.

### Fixed

- `examples/big-cluster/data/accounts/lab_chem.yaml` and `shared.yaml` set
  `default_qos` without an `allowed_qos` list, which Slurm rejects at the
  account level with `HTTP 422: ... would not have access to their default
  qos` — apparently never caught before because the full example had never
  been applied end-to-end in one go. Fixed by adding `allowed_qos` to both.
- `slurm_qos` `usage_factor` and `usage_threshold` now use `Float64`, so
  fractional values (e.g. `0.5`) round-trip without truncation.
- The `slurm_user` data source exposes `association` as a
  `SetNestedAttribute`, fixing schema handling of multi-association users.
- The `slurm_partition` data source tolerates fields that older API versions
  omit: `flags` and `preempt_mode` only exist in the v0.0.45 partition
  schema and are null on older versions instead of failing.
- **`tools/generate_import/generate_import.py`** had drifted from the
  `qos`→`allowed_qos` rename above: its internal `_assoc_fields()` extraction
  and its embedded `generate.tf` template still emitted `qos`, which the
  provider no longer accepts and which the big-cluster `generate.tf` would
  silently drop (falls through `try(m.allowed_qos, null)` to `null`). Fixed,
  and the checked-in `examples/big-cluster/generate.tf` is re-verified
  byte-identical to the generator's output.
- The importer was missing `slurm_user.default_wc_key` entirely (both
  layouts); added, reading `user.default.wckey`. Note: a Slurm REST API
  limitation (verified against a live 26.05.1 cluster; not fixable in this
  project) means `default.wckey` always reads back empty regardless of API
  version, so this field can currently never be populated by the importer
  even when set in Slurm — the provider has the identical read limitation.
  Documented in `tools/generate_import/README.md` and `CLAUDE.md`.
- The importer's `_account_fields()` emitted a spurious `organization` equal
  to the account's own name: Slurm defaults `Organization` to the account
  name when unset (verified via `sacctmgr show account`), indistinguishable
  from an explicit value — same class of issue as the existing `fairshare`
  handling. Now omitted when it matches the account name.
- The importer's `_assoc_fields()` could mistake an **inherited** value for
  an explicit per-user override on five association fields (`default_qos`,
  `max_jobs`, `max_tres_per_job`, `max_tres_per_node`,
  `max_tres_mins_per_job`): verified against a live cluster, Slurm's REST
  API resolves these to the account's own effective value even when the
  user's association never set them, with no flag distinguishing the two
  cases. The importer now omits each of these when it matches the parent
  account's own value (generalizing the anti-inheritance check already
  used for `allowed_qos`). `grp_tres`/`grp_tres_mins`/`grp_tres_run_mins`
  do not inherit this way (also verified) and are unaffected. Without this
  fix, importing a user who merely inherits an account limit would pin
  that value to the user forever, silently breaking future propagation of
  account-limit changes.

### Changed

- The API client is split by entity (`cluster.go`, `account.go`, `qos.go`,
  `user.go`, `assoc.go`, `partition.go`, `conf.go`) instead of one
  monolithic `client.go`.
- `examples/` now contains only end-user-facing content (basic example,
  `big-cluster/`, registry-doc fragments). The seven acceptance-test
  fixture directories moved to `test/fixtures/`; CI workflow paths updated.
- `go.mod` now declares directly-imported deps as direct
  (`terraform-plugin-framework-validators`, `terraform-plugin-go`) and pins
  tfplugindocs once (v0.21.0, chosen for compatibility with the committed
  docs formatting and the go 1.24 toolchain); the Makefile runs it via a
  plain `go run` so the go.mod pin is the single source of truth.

### Removed

- Dead client methods `GetAccounts`, `GetAllQOS`, `GetAssociation`,
  `GetUsers`, `GetClusters`, `DeleteCluster` (no references) and the
  superseded `_account_depth()` in `generate_import.py`. The data sources
  share one `configureDataSourceClient` helper instead of inline copies.

### Verified

- slurmrestd (Slurm 25.05.4) returns HTTP 304 for DELETE on nonexistent
  accounts, users, QOS and associations; the client already treats 304 as
  success, so `Delete` is idempotent when a resource was removed
  out-of-band. Locked in by `TestDoRequest_TreatsNotModifiedAsSuccess` and
  exercised end-to-end (out-of-band delete followed by `tofu destroy`).
