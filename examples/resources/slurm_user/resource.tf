resource "slurm_user" "alice" {
  name            = "alice"
  default_account = slurm_account.physics.name

  association {
    account     = slurm_account.physics.name
    fairshare   = 50
    default_qos = slurm_qos.standard.name
    qos         = [slurm_qos.standard.name, slurm_qos.priority.name]
  }
}

# User with multiple account associations
resource "slurm_user" "bob" {
  name            = "bob"
  default_account = slurm_account.physics.name

  association {
    account   = slurm_account.physics.name
    fairshare = 30
  }

  association {
    account   = slurm_account.hep.name
    fairshare = 20
  }
}

# User with TRES limits on their association
resource "slurm_user" "carol" {
  name            = "carol"
  default_account = slurm_account.gpu_users.name

  association {
    account     = slurm_account.gpu_users.name
    fairshare   = 10
    default_qos = slurm_qos.standard.name
    qos         = [slurm_qos.standard.name, slurm_qos.priority.name]

    # Per-job TRES limits
    max_tres_per_job = [
      { type = "cpu", count = 16 },
      { type = "mem", count = 32768 }, # 32 GB in MB
      { type = "gres", name = "gpu", count = 2 },
    ]

    max_tres_per_node = [
      { type = "gres", name = "gpu", count = 1 },
    ]

    max_tres_mins_per_job = [
      { type = "gres", name = "gpu", count = 2880 }, # 2 GPUs × 24 h
    ]

    # Group-aggregate limits for this user across all their running jobs
    grp_tres = [
      { type = "gres", name = "gpu", count = 4 },
    ]

    grp_tres_mins = [
      { type = "gres", name = "gpu", count = 57600 }, # 40 GPU·h
    ]

    grp_tres_run_mins = [
      { type = "gres", name = "gpu", count = 28800 }, # cap on currently-running GPU·min
    ]
  }
}
