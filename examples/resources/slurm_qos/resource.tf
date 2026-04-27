# Minimal QOS — name and priority only
resource "slurm_qos" "basic" {
  name        = "basic"
  description = "Default priority QOS with no special limits"
  priority    = 100
}

# Wall-clock limits
resource "slurm_qos" "standard" {
  name        = "standard"
  description = "Standard QOS limited to 24 h per job"
  priority    = 200
  max_wall_pj = 1440 # 24 h in minutes
  grp_wall    = 2880 # 48 h total across all running jobs in this QOS
}

# Preemption — this QOS can cancel jobs running under "standard"
resource "slurm_qos" "priority" {
  name        = "priority"
  description = "High-priority QOS that preempts standard jobs"
  priority    = 500
  max_wall_pj = 4320 # 72 h

  preempt_list        = [slurm_qos.standard.name]
  preempt_mode        = ["CANCEL"]
  preempt_exempt_time = 300 # jobs safe from preemption for 5 min after start
}

# CPU and memory TRES limits
resource "slurm_qos" "gpu" {
  name        = "gpu"
  description = "QOS for GPU jobs"
  priority    = 300

  # Per-job TRES limits
  max_tres_per_job = [
    { type = "cpu", count = 128 },
    { type = "mem", count = 512000 }, # 500 GB in MB
    { type = "gres", name = "gpu", count = 8 },
  ]

  # Per-node TRES limits
  max_tres_per_node = [
    { type = "gres", name = "gpu", count = 4 },
  ]

  # Minimum TRES a job must request to use this QOS
  min_tres_per_job = [
    { type = "gres", name = "gpu", count = 1 },
  ]
}

# Group (aggregate) TRES limits shared across all jobs in the QOS
resource "slurm_qos" "shared_gpu_pool" {
  name        = "shared_gpu_pool"
  description = "GPU pool QOS — caps total GPU usage across all users"
  priority    = 250

  grp_tres = [
    { type = "gres", name = "gpu", count = 32 }, # pool of 32 GPUs total
  ]

  grp_tres_mins = [
    { type = "gres", name = "gpu", count = 460800 }, # 32 GPUs × 14400 min (10 d)
  ]
}

# Per-user and per-account TRES limits
resource "slurm_qos" "fairshare" {
  name        = "fairshare"
  description = "QOS enforcing per-user and per-account TRES caps"
  priority    = 150

  max_tres_per_user = [
    { type = "cpu", count = 256 },
    { type = "gres", name = "gpu", count = 16 },
  ]

  max_tres_mins_per_user = [
    { type = "cpu", count = 2880000 }, # ~2000 CPU·h
    { type = "gres", name = "gpu", count = 57600 }, # 40 GPU·h
  ]

  max_tres_per_account = [
    { type = "gres", name = "gpu", count = 64 },
  ]

  max_tres_mins_per_account = [
    { type = "gres", name = "gpu", count = 230400 }, # 160 GPU·h
  ]
}

# Job-count limits
resource "slurm_qos" "burst" {
  name        = "burst"
  description = "Burst QOS — caps the number of running and queued jobs"
  priority    = 100

  grp_jobs        = 500  # total jobs running across all users
  grp_submit_jobs = 2000 # total jobs queued across all users

  max_jobs_per_user        = 50
  max_submit_jobs_per_user = 200

  max_jobs_per_account        = 100
  max_submit_jobs_per_account = 400
}

# Miscellaneous limits
resource "slurm_qos" "scavenger" {
  name        = "scavenger"
  description = "Low-priority scavenger QOS using idle cycles"
  priority    = 10

  usage_factor    = 0 # jobs under this QOS do not consume fairshare
  usage_threshold = 0 # no minimum usage required to submit

  grace_time  = 120 # 2 min before a preempted job is killed
  max_wall_pj = 360 # 6 h limit on scavenger jobs

  flags = ["DENY_LIMIT", "NO_DECAY"]
}
