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
# QOS — five tiers used across association combinations below
# ============================================================================

resource "slurm_qos" "low" {
  name     = "assoc_low"
  priority = 10
}

resource "slurm_qos" "medium" {
  name        = "assoc_medium"
  description = "Medium priority"
  priority    = 100
}

resource "slurm_qos" "high" {
  name        = "assoc_high"
  description = "High priority with wall-clock cap"
  priority    = 500
  max_wall_pj = 1440
}

resource "slurm_qos" "debug" {
  name        = "assoc_debug"
  description = "Short debug jobs"
  max_wall_pj = 60
}

resource "slurm_qos" "long" {
  name        = "assoc_long"
  description = "Long-running jobs"
  max_wall_pj = 10080
}

# ============================================================================
# Accounts — two top-level departments, three child teams
# ============================================================================

# Top-level with full metadata and limits
resource "slurm_account" "dept_a" {
  name         = "dept_a"
  description  = "Department A"
  organization = "org"
  fairshare    = 200
  default_qos  = slurm_qos.medium.name
  allowed_qos  = [slurm_qos.low.name, slurm_qos.medium.name, slurm_qos.high.name]
  max_jobs     = 500
}

# Top-level, minimal — no QOS or job limits
resource "slurm_account" "dept_b" {
  name         = "dept_b"
  description  = "Department B"
  organization = "org"
  fairshare    = 100
}

# Child accounts under dept_a
resource "slurm_account" "team_a1" {
  name           = "team_a1"
  parent_account = slurm_account.dept_a.name
  fairshare      = 80
}

resource "slurm_account" "team_a2" {
  name           = "team_a2"
  parent_account = slurm_account.dept_a.name
  fairshare      = 120
  default_qos    = slurm_qos.high.name
}

# Child account under dept_b
resource "slurm_account" "team_b1" {
  name           = "team_b1"
  parent_account = slurm_account.dept_b.name
  fairshare      = 50
  allowed_qos    = [slurm_qos.low.name, slurm_qos.debug.name]
}

# ============================================================================
# Users
#
# Each user exercises a distinct combination of association fields to verify
# that the provider is idempotent across all optional field combinations.
# The scenarios below cover every subset of {fairshare, default_qos,
# max_jobs, qos} — alone, paired, and in full.
# ============================================================================

# --- Single-association users, one optional field each ---

# No optional fields at all — verifies that Slurm defaults (fairshare=1,
# inherited qos list) are not written back to state and cause drift.
resource "slurm_user" "minimal" {
  name            = "u_minimal"
  default_account = slurm_account.dept_a.name

  association {
    account = slurm_account.dept_a.name
  }
}

# fairshare only
resource "slurm_user" "fairshare_only" {
  name            = "u_fairshare"
  default_account = slurm_account.dept_b.name

  association {
    account   = slurm_account.dept_b.name
    fairshare = 15
  }
}

# default_qos only
resource "slurm_user" "default_qos_only" {
  name            = "u_dqos"
  default_account = slurm_account.dept_a.name

  association {
    account     = slurm_account.dept_a.name
    default_qos = slurm_qos.medium.name
  }
}

# max_jobs only
resource "slurm_user" "max_jobs_only" {
  name            = "u_maxjobs"
  default_account = slurm_account.dept_b.name

  association {
    account  = slurm_account.dept_b.name
    max_jobs = 10
  }
}

# qos list only (no default_qos, no fairshare)
resource "slurm_user" "qos_only" {
  name            = "u_qos"
  default_account = slurm_account.dept_a.name

  association {
    account = slurm_account.dept_a.name
    qos     = [slurm_qos.low.name, slurm_qos.high.name]
  }
}

# --- Single-association users, two optional fields each ---

# fairshare + max_jobs
resource "slurm_user" "fairshare_maxjobs" {
  name            = "u_fs_mj"
  default_account = slurm_account.dept_a.name

  association {
    account   = slurm_account.dept_a.name
    fairshare = 8
    max_jobs  = 20
  }
}

# default_qos + qos list
resource "slurm_user" "qos_pair" {
  name            = "u_qos_pair"
  default_account = slurm_account.dept_a.name

  association {
    account     = slurm_account.dept_a.name
    default_qos = slurm_qos.medium.name
    qos         = [slurm_qos.low.name, slurm_qos.medium.name]
  }
}

# fairshare + default_qos
resource "slurm_user" "fairshare_dqos" {
  name            = "u_fs_dqos"
  default_account = slurm_account.team_a1.name

  association {
    account     = slurm_account.team_a1.name
    fairshare   = 12
    default_qos = slurm_qos.low.name
  }
}

# max_jobs + qos list
resource "slurm_user" "maxjobs_qos" {
  name            = "u_mj_qos"
  default_account = slurm_account.team_b1.name

  association {
    account  = slurm_account.team_b1.name
    max_jobs = 5
    qos      = [slurm_qos.low.name, slurm_qos.debug.name]
  }
}

# --- Single-association, all four optional fields set ---

resource "slurm_user" "all_fields" {
  name            = "u_all"
  default_account = slurm_account.dept_a.name

  association {
    account     = slurm_account.dept_a.name
    fairshare   = 20
    default_qos = slurm_qos.high.name
    max_jobs    = 50
    qos         = [slurm_qos.low.name, slurm_qos.medium.name, slurm_qos.high.name]
  }
}

# --- admin_level tests ---

# admin_level = Operator — exercises the Create two-step (users_association
# ignores administrator_level; provider must call UpdateUser separately).
resource "slurm_user" "operator" {
  name            = "u_operator"
  admin_level     = "Operator"
  default_account = slurm_account.dept_a.name

  association {
    account     = slurm_account.dept_a.name
    fairshare   = 30
    default_qos = slurm_qos.high.name
    max_jobs    = 80
  }
}

# admin_level = None explicitly (same behavior as omitting it, but exercises
# the code path that sends "None" rather than an empty list)
resource "slurm_user" "explicit_none" {
  name            = "u_none"
  admin_level     = "None"
  default_account = slurm_account.dept_b.name

  association {
    account = slurm_account.dept_b.name
  }
}

# --- Multi-association users ---

# Two associations, different field combos per association.
# Exercises the diff algorithm with heterogeneous associations.
resource "slurm_user" "two_accounts" {
  name            = "u_two"
  default_account = slurm_account.dept_a.name

  association {
    account   = slurm_account.dept_a.name
    fairshare = 10
    max_jobs  = 25
    qos       = [slurm_qos.low.name, slurm_qos.medium.name]
  }

  association {
    account     = slurm_account.dept_b.name
    default_qos = slurm_qos.debug.name
  }
}

# Two associations: one full, one minimal.
# The minimal one verifies no inherited-value drift from the full one's account.
resource "slurm_user" "full_and_minimal" {
  name            = "u_full_min"
  default_account = slurm_account.team_a1.name

  association {
    account     = slurm_account.team_a1.name
    fairshare   = 18
    default_qos = slurm_qos.medium.name
    max_jobs    = 35
    qos         = [slurm_qos.low.name, slurm_qos.medium.name]
  }

  association {
    account = slurm_account.dept_b.name
  }
}

# Three associations: one per field group + one minimal.
# Exercises the diff algorithm across three distinct association states.
resource "slurm_user" "three_accounts" {
  name            = "u_three"
  default_account = slurm_account.team_a1.name

  association {
    account   = slurm_account.team_a1.name
    fairshare = 5
    max_jobs  = 15
  }

  association {
    account     = slurm_account.team_a2.name
    default_qos = slurm_qos.high.name
    qos         = [slurm_qos.medium.name, slurm_qos.high.name]
  }

  association {
    account = slurm_account.dept_b.name
  }
}

# Parent + child account associations for the same user.
# Verifies that associations to accounts at different levels of the
# hierarchy don't interfere with each other's state.
resource "slurm_user" "parent_child" {
  name            = "u_hier"
  default_account = slurm_account.team_b1.name

  association {
    account     = slurm_account.team_b1.name
    fairshare   = 25
    default_qos = slurm_qos.low.name
    max_jobs    = 30
    qos         = [slurm_qos.low.name, slurm_qos.debug.name]
  }

  association {
    account  = slurm_account.dept_b.name
    max_jobs = 5
  }
}

# All four optional fields set across all three associations.
# Maximum stress on the diff algorithm and the prior-state map.
resource "slurm_user" "full_limits_multi" {
  name            = "u_full_multi"
  default_account = slurm_account.dept_a.name

  association {
    account     = slurm_account.dept_a.name
    fairshare   = 50
    default_qos = slurm_qos.high.name
    max_jobs    = 100
    qos         = [slurm_qos.low.name, slurm_qos.medium.name, slurm_qos.high.name]
  }

  association {
    account     = slurm_account.team_a1.name
    fairshare   = 20
    default_qos = slurm_qos.medium.name
    max_jobs    = 40
    qos         = [slurm_qos.low.name, slurm_qos.medium.name]
  }

  association {
    account     = slurm_account.team_a2.name
    fairshare   = 30
    default_qos = slurm_qos.low.name
    max_jobs    = 60
    qos         = [slurm_qos.low.name]
  }
}

# Operator with multiple associations covering both departments.
# Exercises admin_level + multi-association simultaneously.
resource "slurm_user" "operator_multi" {
  name            = "u_op_multi"
  admin_level     = "Operator"
  default_account = slurm_account.dept_a.name

  association {
    account     = slurm_account.dept_a.name
    fairshare   = 40
    default_qos = slurm_qos.high.name
    qos         = [slurm_qos.medium.name, slurm_qos.high.name]
  }

  association {
    account   = slurm_account.dept_b.name
    max_jobs  = 20
  }

  association {
    account     = slurm_account.team_a2.name
    fairshare   = 10
    default_qos = slurm_qos.medium.name
    max_jobs    = 15
    qos         = [slurm_qos.low.name, slurm_qos.medium.name]
  }
}
