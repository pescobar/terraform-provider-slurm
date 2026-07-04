// conf-datasource-tests/main.tf
//
// Fixture for the v0.0.45-only configuration data sources `data.slurm_conf`
// and `data.slurm_dbd_conf`. The CI step running this fixture is gated on
// matrix.api_version == v0.0.45 — on older API versions the data sources
// fail (by design) with a "requires Slurm 26.05+" plan-time error, which is
// covered by the version-error step instead.
//
// Run:
//   cd test/fixtures/conf-datasource-tests
//   tofu apply -auto-approve -var "slurm_token=$SLURM_JWT_TOKEN"
//   tofu output -json

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
  default = "v0.0.45"
}

data "slurm_conf" "cluster" {}

data "slurm_dbd_conf" "accounting" {}

# ---------- Outputs the CI step asserts on ----------

output "slurm_version" { value = data.slurm_conf.cluster.slurm_version }
output "cluster_name" { value = data.slurm_conf.cluster.cluster_name }
output "conf_path" { value = data.slurm_conf.cluster.conf_path }
output "scheduler_type" { value = data.slurm_conf.cluster.conf["SchedulerType"] }

# The conf map must carry the full configuration, not a subset. 200 is a
# loose lower bound (26.05.1 returns 220 keys) so new Slurm releases that
# add or remove a few keys don't break the assertion.
output "conf_is_complete" { value = length(data.slurm_conf.cluster.conf) >= 200 }

output "dbd_track_wckey" { value = data.slurm_dbd_conf.accounting.conf["TrackWCKey"] }
output "dbd_is_complete" { value = length(data.slurm_dbd_conf.accounting.conf) >= 25 }
