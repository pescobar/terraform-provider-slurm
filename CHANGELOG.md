# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `examples/big-cluster/`: a data-driven, human-friendly layout for managing
  large clusters (hundreds of accounts, thousands of users). Sysadmins edit
  account-centric YAML under `data/` (one file per account with its member
  list, plus a small `users.yaml` for exceptions), and `generate.tf` inverts
  it into the user-centric `slurm_user`/`slurm_account` resources via
  `for_each`. Includes a README with a daily-ops cheat sheet and a worked
  multi-account user example.
