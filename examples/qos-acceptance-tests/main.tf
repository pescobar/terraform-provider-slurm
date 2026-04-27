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
# qtest_minimal — name only
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
# qtest_preempt — preemption (list + mode + exempt time)
# ============================================================================

resource "slurm_qos" "qtest_preempt" {
  name        = "qtest_preempt"
  description = "Preemption QOS"
  priority    = 900

  preempt_list        = [slurm_qos.qtest_minimal.name, slurm_qos.qtest_wall.name]
  preempt_mode        = ["CANCEL"]
  preempt_exempt_time = 300 # 5 min
}

# ============================================================================
# qtest_tres_job — TRES limits per job (cpu + mem only; no gres since the
# test cluster does not have GPUs configured in slurm.conf/gres.conf)
# ============================================================================

resource "slurm_qos" "qtest_tres_job" {
  name        = "qtest_tres_job"
  description = "Per-job TRES limits"
  priority    = 200

  max_tres_per_job = [
    { type = "cpu", count = 64 },
    { type = "mem", count = 262144 }, # 256 GB in MB
  ]
}

# ============================================================================
# qtest_tres_grp — aggregate TRES limits (cpu + mem)
# ============================================================================

resource "slurm_qos" "qtest_tres_grp" {
  name        = "qtest_tres_grp"
  description = "Group TRES limits"
  priority    = 150

  grp_tres = [
    { type = "cpu", count = 256 },
    { type = "mem", count = 524288 }, # 512 GB in MB
  ]

  grp_tres_mins = [
    { type = "cpu", count = 1536000 }, # ~1066 CPU·h
  ]
}

# ============================================================================
# qtest_tres_user — per-user TRES limits (cpu + mem)
# ============================================================================

resource "slurm_qos" "qtest_tres_user" {
  name        = "qtest_tres_user"
  description = "Per-user TRES limits"
  priority    = 160

  max_tres_per_user = [
    { type = "cpu", count = 128 },
    { type = "mem", count = 131072 }, # 128 GB in MB
  ]

  max_tres_mins_per_user = [
    { type = "cpu", count = 768000 }, # ~533 CPU·h
  ]
}

# ============================================================================
# qtest_tres_acct — per-account TRES limits (cpu + mem)
# ============================================================================

resource "slurm_qos" "qtest_tres_acct" {
  name        = "qtest_tres_acct"
  description = "Per-account TRES limits"
  priority    = 170

  max_tres_per_account = [
    { type = "cpu", count = 512 },
  ]

  max_tres_mins_per_account = [
    { type = "cpu", count = 3072000 }, # ~2133 CPU·h
  ]
}

# ============================================================================
# qtest_jobs — all six job-count limits
# ============================================================================

resource "slurm_qos" "qtest_jobs" {
  name        = "qtest_jobs"
  description = "Job-count limit QOS"
  priority    = 100

  grp_jobs        = 200
  grp_submit_jobs = 800

  max_jobs_per_user        = 20
  max_submit_jobs_per_user = 80

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

  grace_time      = 60
  usage_factor    = 2
  usage_threshold = 100

  max_wall_pj = 360

  flags = ["NO_DECAY", "DENY_LIMIT"]
}

# ============================================================================
# qtest_tres_mins_job — per-job TRES-minutes limit (MaxTRESMins)
# ============================================================================

resource "slurm_qos" "qtest_tres_mins_job" {
  name        = "qtest_tres_mins_job"
  description = "Per-job TRES-minutes limit"
  priority    = 180

  max_tres_mins_per_job = [
    { type = "cpu", count = 86400 }, # 1440 CPU·min per job (24 CPU·h)
  ]
}
