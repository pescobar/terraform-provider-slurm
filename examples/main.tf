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
  api_version = "v0.0.42"
}

variable "slurm_token" {
  type      = string
  sensitive = true
}

resource "slurm_qos" "standard" {
  name        = "standard"
  description = "Standard priority QOS"
  priority    = 100
  max_wall_pj = 1440
}

resource "slurm_qos" "priority" {
  name        = "priority"
  description = "High priority QOS"
  priority    = 200
  max_wall_pj = 2880

  preempt_list = [slurm_qos.standard.name]
  preempt_mode = ["CANCEL"]
}

resource "slurm_account" "physics" {
  name           = "physics"
  description    = "Physics department"
  organization   = "university"
  fairshare      = 100
  default_qos    = slurm_qos.standard.name
  allowed_qos    = [slurm_qos.standard.name, slurm_qos.priority.name]
}

resource "slurm_user" "bob" {
  name            = "bob"
  default_account = slurm_account.physics.name

  association {
    account = slurm_account.physics.name
  }
}
