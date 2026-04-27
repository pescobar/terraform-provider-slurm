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
# qtest_minimal — name only; verifies Slurm creates a QOS with all defaults
# ============================================================================

resource "slurm_qos" "qtest_minimal" {
  name = "qtest_minimal"
}

# ============================================================================
# qtest_wall — wall-clock limits (MaxWall + GrpWall)
# ============================================================================

resource "slurm_qos" "qtest_wall" {
  name        = "qtest_wall"
  description = "Wall-clock limit QOS"
  priority    = 50
  max_wall_pj = 720  # 12 h per job
  grp_wall    = 1440 # 24 h total across all running jobs
}

# ============================================================================
# qtest_preempt — preemption config (list + mode + exempt time)
# ============================================================================

resource "slurm_qos" "qtest_preempt" {
  name        = "qtest_preempt"
  description = "Preemption QOS"
  priority    = 900

  preempt_list        = [slurm_qos.qtest_minimal.name, slurm_qos.qtest_wall.name]
  preempt_mode        = ["CANCEL"]
  preempt_exempt_time = 300 # 5 min before a job can be preempted
}

# ============================================================================
# qtest_tres_job — TRES limits per-job and per-node, plus minimum TRES
# ============================================================================

resource "slurm_qos" "qtest_tres_job" {
  name        = "qtest_tres_job"
  description = "Per-job and per-node TRES limits"
  priority    = 200

  # Maximum TRES a single job may request
  max_tres_per_job = [
    { type = "cpu", count = 64 },
    { type = "mem", count = 256000 }, # 250 GB in MB
    { type = "gres", name = "gpu", count = 4 },
  ]

  # Maximum TRES a single job may use on any one node
  max_tres_per_node = [
    { type = "gres", name = "gpu", count = 2 },
  ]

  # Minimum TRES a job must request to use this QOS
  min_tres_per_job = [
    { type = "gres", name = "gpu", count = 1 },
  ]
}

# ============================================================================
# qtest_tres_grp — aggregate (group) TRES limits shared across all jobs
# ============================================================================

resource "slurm_qos" "qtest_tres_grp" {
  name        = "qtest_tres_grp"
  description = "Group TRES and TRES-minutes limits"
  priority    = 150

  # Total resources usable by all jobs in this QOS at the same time
  grp_tres = [
    { type = "cpu", count = 256 },
    { type = "gres", name = "gpu", count = 16 },
  ]

  # Cumulative TRES-minutes budget for all jobs in this QOS
  grp_tres_mins = [
    { type = "cpu", count = 1536000 }, # ~1066 CPU·h
    { type = "gres", name = "gpu", count = 57600 },  # 40 GPU·h
  ]
}

# ============================================================================
# qtest_tres_user — per-user TRES limits (instantaneous + cumulative)
# ============================================================================

resource "slurm_qos" "qtest_tres_user" {
  name        = "qtest_tres_user"
  description = "Per-user TRES limits"
  priority    = 160

  # Maximum resources one user can use simultaneously
  max_tres_per_user = [
    { type = "cpu", count = 128 },
    { type = "gres", name = "gpu", count = 8 },
  ]

  # Maximum cumulative TRES-minutes one user can consume
  max_tres_mins_per_user = [
    { type = "cpu", count = 768000 },  # ~533 CPU·h
    { type = "gres", name = "gpu", count = 28800 }, # 20 GPU·h
  ]
}

# ============================================================================
# qtest_tres_acct — per-account TRES limits (instantaneous + cumulative)
# ============================================================================

resource "slurm_qos" "qtest_tres_acct" {
  name        = "qtest_tres_acct"
  description = "Per-account TRES limits"
  priority    = 170

  # Maximum resources one account can use simultaneously
  max_tres_per_account = [
    { type = "cpu", count = 512 },
    { type = "gres", name = "gpu", count = 32 },
  ]

  # Maximum cumulative TRES-minutes one account can consume
  max_tres_mins_per_account = [
    { type = "gres", name = "gpu", count = 115200 }, # 80 GPU·h
  ]
}

# ============================================================================
# qtest_jobs — all job-count limits
# ============================================================================

resource "slurm_qos" "qtest_jobs" {
  name        = "qtest_jobs"
  description = "Job-count limit QOS"
  priority    = 100

  # Group limits (across all users)
  grp_jobs        = 200  # total running jobs in this QOS
  grp_submit_jobs = 800  # total queued jobs in this QOS

  # Per-user limits
  max_jobs_per_user        = 20
  max_submit_jobs_per_user = 80

  # Per-account limits
  max_jobs_per_account        = 40
  max_submit_jobs_per_account = 160
}

# ============================================================================
# qtest_misc — grace_time, usage_factor, usage_threshold, flags
# ============================================================================

resource "slurm_qos" "qtest_misc" {
  name        = "qtest_misc"
  description = "Miscellaneous limit QOS"
  priority    = 10

  grace_time      = 60  # 60 s before preempted job is killed
  usage_factor    = 2   # jobs consume fairshare at 2× rate
  usage_threshold = 100 # minimum usage score required to submit

  max_wall_pj = 360 # 6 h

  flags = ["NO_DECAY", "DENY_LIMIT"]
}

# ============================================================================
# qtest_tres_mins_job — per-job TRES-minutes limit (MaxTRESMins)
# Separate resource to keep each QOS focused on one feature group.
# ============================================================================

resource "slurm_qos" "qtest_tres_mins_job" {
  name        = "qtest_tres_mins_job"
  description = "Per-job TRES-minutes limit"
  priority    = 180

  max_tres_mins_per_job = [
    { type = "cpu", count = 86400 },  # 1440 CPU·min per job (24 CPU·h)
    { type = "gres", name = "gpu", count = 2880 }, # 48 GPU·h per job
  ]
}
