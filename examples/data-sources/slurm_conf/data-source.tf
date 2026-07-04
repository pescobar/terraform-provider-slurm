# Read the active slurmctld configuration — the live equivalent of
# `scontrol show config`. Requires Slurm 26.05+ (API version v0.0.45+).
data "slurm_conf" "cluster" {}

output "running_slurm_version" {
  value = data.slurm_conf.cluster.slurm_version
}

# Preflight assertion: warn when the cluster is not enforcing associations —
# the accounting model this provider manages assumes it.
check "accounting_enforce_includes_associations" {
  assert {
    condition     = strcontains(data.slurm_conf.cluster.conf["AccountingStorageEnforce"], "associations")
    error_message = "AccountingStorageEnforce must include 'associations'."
  }
}
