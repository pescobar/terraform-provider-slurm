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
# Account — shared by all users in this test suite
# ============================================================================

resource "slurm_account" "lim" {
  name        = "alim"
  description = "Account for association limit acceptance tests"
}

# ============================================================================
# uat_minimal — no limit fields; verifies that Slurm defaults (fairshare=1,
# inherited values) do not surface as drift.
# ============================================================================

resource "slurm_user" "uat_minimal" {
  name            = "uat_minimal"
  default_account = slurm_account.lim.name

  association {
    account = slurm_account.lim.name
  }
}

# ============================================================================
# uat_job_counts — all six job-count limits
# ============================================================================

resource "slurm_user" "uat_job_counts" {
  name            = "uat_job_counts"
  default_account = slurm_account.lim.name

  association {
    account         = slurm_account.lim.name
    max_jobs        = 5
    max_jobs_accrue = 10
    max_submit_jobs = 20
    grp_jobs        = 100
    grp_jobs_accrue = 200
    grp_submit_jobs = 400
  }
}

# ============================================================================
# uat_wall — wall-clock limits (per-job and group aggregate)
# ============================================================================

resource "slurm_user" "uat_wall" {
  name            = "uat_wall"
  default_account = slurm_account.lim.name

  association {
    account     = slurm_account.lim.name
    max_wall_pj = 60    # 1 hour per job
    grp_wall    = 1440  # 1 day aggregate
  }
}

# ============================================================================
# uat_max_tres — per-job and per-node TRES limits (cpu + mem)
# ============================================================================

resource "slurm_user" "uat_max_tres" {
  name            = "uat_max_tres"
  default_account = slurm_account.lim.name

  association {
    account = slurm_account.lim.name

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
  }
}

# ============================================================================
# uat_grp_tres — group-aggregate TRES limits (cpu only)
# ============================================================================

resource "slurm_user" "uat_grp_tres" {
  name            = "uat_grp_tres"
  default_account = slurm_account.lim.name

  association {
    account = slurm_account.lim.name

    grp_tres = [
      { type = "cpu", count = 256 },
    ]

    grp_tres_mins = [
      { type = "cpu", count = 153600 }, # ~106 CPU·h
    ]

    grp_tres_run_mins = [
      { type = "cpu", count = 76800 }, # ~53 CPU·h running
    ]
  }
}

# ============================================================================
# uat_priority — association-level priority
# ============================================================================

resource "slurm_user" "uat_priority" {
  name            = "uat_priority"
  default_account = slurm_account.lim.name

  association {
    account  = slurm_account.lim.name
    priority = 10
  }
}

# ============================================================================
# uat_fairshare_priority — fairshare + priority together
# ============================================================================

resource "slurm_user" "uat_fairshare_priority" {
  name            = "uat_fs_prio"
  default_account = slurm_account.lim.name

  association {
    account   = slurm_account.lim.name
    fairshare = 50
    priority  = 5
  }
}

# ============================================================================
# uat_all_limits — every new limit field set in a single association
# (the kitchen-sink test — exercises the full extractAssocMax and
# apiAssociationsToState code paths simultaneously)
# ============================================================================

resource "slurm_user" "uat_all_limits" {
  name            = "uat_all_lim"
  default_account = slurm_account.lim.name

  association {
    account   = slurm_account.lim.name
    fairshare = 50
    priority  = 10

    # Job-count limits
    max_jobs        = 5
    max_jobs_accrue = 10
    max_submit_jobs = 20
    grp_jobs        = 100
    grp_jobs_accrue = 200
    grp_submit_jobs = 400

    # Wall-clock limits
    max_wall_pj = 60    # 1 hour per job
    grp_wall    = 1440  # 1 day aggregate

    # Per-job and per-node TRES
    max_tres_per_job = [
      { type = "cpu", count = 8 },
      { type = "mem", count = 16384 },
    ]
    max_tres_per_node = [
      { type = "cpu", count = 4 },
    ]
    max_tres_mins_per_job = [
      { type = "cpu", count = 480 },
    ]

    # Group-aggregate TRES
    grp_tres = [
      { type = "cpu", count = 256 },
    ]
    grp_tres_mins = [
      { type = "cpu", count = 153600 },
    ]
    grp_tres_run_mins = [
      { type = "cpu", count = 76800 },
    ]
  }
}

# ============================================================================
# uat_multi_assoc — two associations on the same user, each with a different
# subset of the new limit fields; exercises the diff algorithm with
# heterogeneous new-field combinations.
# ============================================================================

resource "slurm_user" "uat_multi_assoc" {
  name            = "uat_multi"
  default_account = slurm_account.lim.name

  # Primary association: job-count + wall-clock limits
  association {
    account         = slurm_account.lim.name
    fairshare       = 30
    max_jobs        = 10
    max_submit_jobs = 40
    max_wall_pj     = 120
    grp_jobs        = 50
    grp_wall        = 720
  }

  # Secondary association under root: TRES limits only
  association {
    account = "root"

    max_tres_per_job = [
      { type = "cpu", count = 16 },
    ]
    grp_tres = [
      { type = "cpu", count = 128 },
    ]
  }
}
