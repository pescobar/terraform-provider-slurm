// system-qos-warning/main.tf
//
// Negative-case fixture for the slurm_qos ValidateConfig warning that
// fires when a user manages one of Slurm's auto-created system QOS rows
// (currently just "normal"). Bug 3 in CLAUDE.md documents the failure
// mode this warning is meant to surface before users hit it on the
// second apply.
//
// Run:
//   cd test/fixtures/system-qos-warning
//   tofu plan -var "slurm_token=$SLURM_JWT_TOKEN"
//
// Expected: tofu plan exits 0 (warnings do not fail plan) and prints a
// "Warning: Managing built-in system QOS …" diagnostic for the
// slurm_qos.system_normal block. There is nothing to apply or destroy.

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

# Names a built-in system QOS — expect a Warning, not an Error.
resource "slurm_qos" "system_normal" {
  name     = "normal"
  priority = 100
}
