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

- `slurm_qos` `usage_factor` and `usage_threshold` now use `Float64`, so
  fractional values (e.g. `0.5`) round-trip without truncation.
- The `slurm_user` data source exposes `association` as a
  `SetNestedAttribute`, fixing schema handling of multi-association users.
- The `slurm_partition` data source tolerates fields that older API versions
  omit: `flags` and `preempt_mode` only exist in the v0.0.45 partition
  schema and are null on older versions instead of failing.

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
