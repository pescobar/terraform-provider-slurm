# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

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
