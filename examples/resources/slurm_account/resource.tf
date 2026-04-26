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
