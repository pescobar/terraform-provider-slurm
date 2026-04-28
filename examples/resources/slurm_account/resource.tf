resource "slurm_account" "physics" {
  name         = "physics"
  description  = "Physics department"
  organization = "university"
  fairshare    = 100
  default_qos  = slurm_qos.standard.name
  allowed_qos  = [slurm_qos.standard.name, slurm_qos.priority.name]
}

# Child account inheriting from physics
resource "slurm_account" "hep" {
  name           = "hep"
  description    = "High Energy Physics group"
  organization   = "university"
  parent_account = slurm_account.physics.name
  fairshare      = 50
}

# Account with per-job and group TRES limits
resource "slurm_account" "gpu_users" {
  name        = "gpu_users"
  description = "GPU users group"

  # Per-job limits
  max_tres_per_job = [
    { type = "cpu", count = 64 },
    { type = "mem", count = 131072 }, # 128 GB in MB
    { type = "gres", name = "gpu", count = 8 },
  ]

  max_tres_per_node = [
    { type = "gres", name = "gpu", count = 4 },
  ]

  # Group-aggregate limits shared across all running jobs in this account
  grp_tres = [
    { type = "gres", name = "gpu", count = 32 },
  ]

  grp_tres_mins = [
    { type = "gres", name = "gpu", count = 460800 }, # 32 GPUs × 14400 min (10 d)
  ]

  grp_tres_run_mins = [
    { type = "gres", name = "gpu", count = 230400 }, # cap on currently-running GPU·min
  ]
}
