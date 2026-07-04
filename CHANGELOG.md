# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

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

### Changed

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
  `GetUsers` (no references) and the superseded `_account_depth()` in
  `generate_import.py`. The three data sources now share one
  `configureDataSourceClient` helper instead of inline copies.

### Verified

- slurmrestd (Slurm 25.05.4) returns HTTP 304 for DELETE on nonexistent
  accounts, users, QOS and associations; the client already treats 304 as
  success, so `Delete` is idempotent when a resource was removed
  out-of-band. Locked in by `TestDoRequest_TreatsNotModifiedAsSuccess` and
  exercised end-to-end (out-of-band delete followed by `tofu destroy`).
