terraform {
  required_providers {
    slurm = {
      source = "pescobar/slurm"
    }
  }
}

locals {
  cluster_name = "linux"
}

provider "slurm" {
  endpoint    = "http://localhost:6820"
  token       = var.slurm_token
  cluster     = local.cluster_name
  api_version = "v0.0.42"
}

variable "slurm_token" {
  type      = string
  sensitive = true
}

resource "slurm_cluster" "main" {
  name = local.cluster_name
}

resource "slurm_qos" "normal" {
  name        = "normal"
  description = "Normal priority QOS"
  priority    = 100
  max_wall_pj = 1440  # 24 hours in minutes
}

resource "slurm_qos" "high" {
  name        = "high"
  description = "High priority QOS"
  priority    = 200
  max_wall_pj = 2880  # 48 hours in minutes

  preempt_list = [slurm_qos.normal.name]
  preempt_mode = ["CANCEL"]
}

resource "slurm_account" "physics" {
  name           = "physics"
  description    = "Physics department"
  organization   = "university"
  fairshare      = 100
  default_qos    = slurm_qos.normal.name
  allowed_qos    = [slurm_qos.normal.name, slurm_qos.high.name]
}

resource "slurm_user" "bob" {
  name            = "bob"
  default_account = slurm_account.physics.name
}
