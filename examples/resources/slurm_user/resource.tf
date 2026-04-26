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
