// data-source-tests/notfound/main.tf
//
// Negative fixture for the data sources: every block references a name
// that does not exist in Slurm. `tofu plan` is expected to fail with a
// "<entity> not found" diagnostic per data source. No resources are
// declared, so there is nothing to apply or destroy.

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

# All three names are guaranteed not to exist alongside the live
# cluster's seed data.
data "slurm_qos" "missing" {
  name = "ds_qos_does_not_exist"
}

data "slurm_account" "missing" {
  name = "ds_acct_does_not_exist"
}

data "slurm_user" "missing" {
  name = "ds_user_does_not_exist"
}
