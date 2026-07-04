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

# ============================================================================
# QOS
# Exercises: name only, description, priority, max_wall_pj, flags,
#            preempt_list, preempt_mode — in various combinations.
# ============================================================================

# Minimal — name only
resource "slurm_qos" "basic" {
  name = "basic"
}

# Description + priority + wall clock limit
resource "slurm_qos" "standard" {
  name        = "standard"
  description = "Standard QOS for normal jobs"
  priority    = 100
  max_wall_pj = 1440
}

# All fields — flags, preemption of lower QOS
resource "slurm_qos" "priority" {
  name         = "priority"
  description  = "High priority QOS with preemption"
  priority     = 500
  max_wall_pj  = 720
  flags        = ["NO_DECAY", "DENY_LIMIT"]
  preempt_list = [slurm_qos.standard.name, slurm_qos.basic.name]
  preempt_mode = ["CANCEL"]
}

# Priority only
resource "slurm_qos" "express" {
  name     = "express"
  priority = 1000
}

# Flags + short wall time
resource "slurm_qos" "debug" {
  name        = "debug"
  description = "Debug QOS with short wall time"
  max_wall_pj = 60
  flags       = ["OVERRIDE_PARTITION_QOS"]
}

# ============================================================================
# Accounts
# Exercises: name only, description, organization, parent_account,
#            fairshare, default_qos, allowed_qos, max_jobs — in various
#            combinations including parent-child relationships.
# ============================================================================

# Minimal — name only
resource "slurm_account" "minimal" {
  name = "minimal"
}

# All fields — top-level account
resource "slurm_account" "physics" {
  name         = "physics"
  description  = "Physics department"
  organization = "university"
  fairshare    = 100
  default_qos  = slurm_qos.standard.name
  allowed_qos  = [slurm_qos.basic.name, slurm_qos.standard.name, slurm_qos.priority.name]
  max_jobs     = 200
}

# Top-level, subset of fields
resource "slurm_account" "chemistry" {
  name         = "chemistry"
  description  = "Chemistry department"
  organization = "university"
  fairshare    = 50
  default_qos  = slurm_qos.basic.name
  allowed_qos  = [slurm_qos.basic.name, slurm_qos.standard.name]
}

# Child account — all fields, parent_account set
resource "slurm_account" "physics_hep" {
  name           = "physics_hep"
  description    = "High Energy Physics subgroup"
  organization   = "university"
  parent_account = slurm_account.physics.name
  fairshare      = 40
  default_qos    = slurm_qos.priority.name
  allowed_qos    = [slurm_qos.standard.name, slurm_qos.priority.name, slurm_qos.express.name]
  max_jobs       = 50
}

# Child account — minimal fields + parent_account
resource "slurm_account" "physics_astro" {
  name           = "physics_astro"
  description    = "Astrophysics subgroup"
  parent_account = slurm_account.physics.name
  fairshare      = 60
  allowed_qos    = [slurm_qos.debug.name]
}

# Account with a subset of TRES limits
resource "slurm_account" "tres_partial" {
  name        = "tres_partial"
  description = "Account with per-job and group TRES limits"

  max_tres_per_job = [
    { type = "cpu", count = 32 },
    { type = "mem", count = 65536 }, # 64 GB in MB
  ]

  grp_tres = [
    { type = "cpu", count = 256 },
  ]

  grp_tres_mins = [
    { type = "cpu", count = 153600 }, # ~106 CPU·h
  ]
}

# Account with all six TRES limit fields
resource "slurm_account" "tres_all" {
  name        = "tres_all"
  description = "Account with all TRES limit fields set"

  max_tres_per_job = [
    { type = "cpu", count = 8 },
    { type = "mem", count = 16384 }, # 16 GB in MB
  ]

  max_tres_per_node = [
    { type = "cpu", count = 4 },
  ]

  max_tres_mins_per_job = [
    { type = "cpu", count = 480 }, # 8 CPU·h
  ]

  grp_tres = [
    { type = "cpu", count = 64 },
  ]

  grp_tres_mins = [
    { type = "cpu", count = 38400 }, # ~26 CPU·h
  ]

  grp_tres_run_mins = [
    { type = "cpu", count = 19200 }, # ~13 CPU·h running
  ]
}

# ============================================================================
# Users
# Exercises: name, admin_level (None/Operator), default_account, and
#            association blocks with all combinations of account, fairshare,
#            default_qos, max_jobs, qos — including multiple associations
#            per user across different accounts.
# ============================================================================

# Minimal user — single association, no optional fields
resource "slurm_user" "alice" {
  name            = "alice"
  default_account = slurm_account.minimal.name

  association {
    account = slurm_account.minimal.name
  }
}

# Single association with all association fields set
resource "slurm_user" "bob" {
  name            = "bob"
  admin_level     = "None"
  default_account = slurm_account.physics.name

  association {
    account     = slurm_account.physics.name
    fairshare   = 10
    default_qos = slurm_qos.standard.name
    max_jobs    = 20
    qos         = [slurm_qos.basic.name, slurm_qos.standard.name]
  }
}

# Operator admin level, three associations across different accounts
resource "slurm_user" "carol" {
  name            = "carol"
  admin_level     = "Operator"
  default_account = slurm_account.physics.name

  association {
    account     = slurm_account.physics.name
    fairshare   = 20
    default_qos = slurm_qos.standard.name
    qos         = [slurm_qos.basic.name, slurm_qos.standard.name, slurm_qos.priority.name]
  }

  association {
    account     = slurm_account.chemistry.name
    fairshare   = 5
    default_qos = slurm_qos.basic.name
  }

  association {
    account  = slurm_account.physics_hep.name
    max_jobs = 10
    qos      = [slurm_qos.priority.name, slurm_qos.express.name]
  }
}

# Two associations — one with limits, one with debug QOS in child account
resource "slurm_user" "dave" {
  name            = "dave"
  default_account = slurm_account.chemistry.name

  association {
    account   = slurm_account.chemistry.name
    fairshare = 8
    max_jobs  = 5
  }

  association {
    account     = slurm_account.physics_astro.name
    default_qos = slurm_qos.debug.name
    max_jobs    = 3
  }
}
