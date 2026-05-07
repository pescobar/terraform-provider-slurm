# Look up an existing Slurm account by name. Returns its metadata and
# the limits attached to the account-level association — fairshare,
# default QOS, allowed QOS, max running jobs, and TRES limits.

data "slurm_account" "physics" {
  name = "physics"
}

# Sub-account that inherits the parent's allowed QOS list.
resource "slurm_account" "physics_hep" {
  name           = "physics_hep"
  parent_account = data.slurm_account.physics.name
  allowed_qos    = data.slurm_account.physics.allowed_qos
}
