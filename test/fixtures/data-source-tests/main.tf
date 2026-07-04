// data-source-tests/main.tf
//
// End-to-end fixture for the read-only data sources `data.slurm_qos`,
// `data.slurm_account`, and `data.slurm_user`. Each resource is created
// alongside a matching data source that reads it back by name; outputs
// surface key attributes so the CI step can grep for ground-truth values.
//
// Run:
//   cd test/fixtures/data-source-tests
//   tofu apply -auto-approve -var "slurm_token=$SLURM_JWT_TOKEN"
//   tofu output -json
//   tofu destroy -auto-approve -var "slurm_token=$SLURM_JWT_TOKEN"

terraform {
  required_providers {
    slurm = {
      source = "pescobar/slurm"
    }
  }
}

provider "slurm" {
  endpoint    = "http://localhost:6820"
  token       = var.slurm_token
  cluster     = "linux"
  api_version = var.slurm_api_version
}

variable "slurm_token" {
  type      = string
  sensitive = true
}

variable "slurm_api_version" {
  type    = string
  default = "v0.0.42"
}

# ---------- Managed resources ----------

resource "slurm_qos" "ds_qos" {
  name        = "ds_qos"
  description = "QOS exercised by data-source-tests"
  priority    = 250
  max_wall_pj = 720
}

resource "slurm_account" "ds_acct" {
  name        = "ds_acct"
  description = "Account exercised by data-source-tests"
  fairshare   = 4
  max_jobs    = 50
}

resource "slurm_account" "ds_acct_alt" {
  name      = "ds_acct_alt"
  fairshare = 2
}

resource "slurm_user" "ds_user" {
  name            = "ds_user"
  default_account = "ds_acct"
  admin_level     = "Operator"

  association {
    account   = slurm_account.ds_acct.name
    fairshare = 7
    max_jobs  = 12
  }
}

# Two-association user — exercises that the data-source SetNestedBlock
# round-trips multiple entries, not just one.
resource "slurm_user" "ds_user_multi" {
  name            = "ds_user_multi"
  default_account = "ds_acct"

  association {
    account   = slurm_account.ds_acct.name
    fairshare = 5
  }
  association {
    account   = slurm_account.ds_acct_alt.name
    fairshare = 3
  }
}

# ---------- Data sources reading the resources back ----------

data "slurm_qos" "by_name" {
  name       = slurm_qos.ds_qos.name
  depends_on = [slurm_qos.ds_qos]
}

data "slurm_account" "by_name" {
  name       = slurm_account.ds_acct.name
  depends_on = [slurm_account.ds_acct]
}

data "slurm_user" "by_name" {
  name       = slurm_user.ds_user.name
  depends_on = [slurm_user.ds_user]
}

data "slurm_user" "by_name_multi" {
  name       = slurm_user.ds_user_multi.name
  depends_on = [slurm_user.ds_user_multi]
}

# ---------- Outputs the CI step asserts on ----------

output "qos_priority" { value = data.slurm_qos.by_name.priority }
output "qos_max_wall_pj" { value = data.slurm_qos.by_name.max_wall_pj }
output "qos_description" { value = data.slurm_qos.by_name.description }

output "account_fairshare" { value = data.slurm_account.by_name.fairshare }
output "account_max_jobs" { value = data.slurm_account.by_name.max_jobs }

output "user_default_account" { value = data.slurm_user.by_name.default_account }
output "user_admin_level" { value = data.slurm_user.by_name.admin_level }
output "user_association_count" { value = length(data.slurm_user.by_name.association) }

# Filter the SetNestedBlock by account to extract a deterministic value —
# Set ordering is not stable across reads, so indexing by [0] would be
# brittle. The [0] applied to the filtered list is safe because each user
# has at most one association per (account, partition) pair.
output "user_assoc_fairshare" {
  value = [for a in data.slurm_user.by_name.association : a.fairshare if a.account == "ds_acct"][0]
}
output "user_assoc_max_jobs" {
  value = [for a in data.slurm_user.by_name.association : a.max_jobs if a.account == "ds_acct"][0]
}

# Multi-association user — confirms the data source returns *all* associations,
# not just one. A second filter pulls the alt-account fairshare for an extra
# spot-check on the per-block decoding.
output "user_multi_assoc_count" {
  value = length(data.slurm_user.by_name_multi.association)
}
output "user_multi_alt_fairshare" {
  value = [for a in data.slurm_user.by_name_multi.association : a.fairshare if a.account == "ds_acct_alt"][0]
}

# ---------- Partition data source (read-only cluster state) ----------
# The "cpu" partition is defined in the test image's slurm.conf, so this
# works on every Slurm version in the CI matrix (partition GET has existed
# since v0.0.42). The conf data sources are v0.0.45+ and live in the
# separate conf-datasource-tests fixture.

data "slurm_partition" "cpu" {
  name = "cpu"
}

output "partition_name" { value = data.slurm_partition.cpu.name }
output "partition_state" { value = join(",", data.slurm_partition.cpu.state) }

# flags only exists in the v0.0.45+ partition schema (Slurm 26.05); the data
# source returns null on older versions, so the join must be null-tolerant.
# The CI step asserts "DEFAULT" on v0.0.45 and "" on older matrix entries.
output "partition_flags" {
  value = try(join(",", data.slurm_partition.cpu.flags), "")
}
