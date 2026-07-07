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

# Deliberately leaves every optional attribute unset. slurm_account's fields
# are Optional-only with null-preservation (see docs/resources/account.md):
# Read() only writes an Optional field into state when the prior state value
# is already non-null. Since a fresh import starts with an empty state, a
# clean import followed immediately by `tofu plan` shows no diff -- this
# fixture never sets, and therefore never risks changing, root's real
# fairshare/default_qos/allowed_qos.
resource "slurm_account" "root_protected" {
  name = "root"
}
