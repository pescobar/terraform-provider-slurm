# Read the active slurmdbd configuration — the live equivalent of
# `sacctmgr show config`. Requires Slurm 26.05+ (API version v0.0.45+).
data "slurm_dbd_conf" "accounting" {}

output "wckey_tracking_enabled" {
  value = data.slurm_dbd_conf.accounting.conf["TrackWCKey"]
}
