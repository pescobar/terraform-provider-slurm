# Look up an existing Slurm QOS by name and reference its attributes
# without bringing it under provider management. Useful when the QOS is
# maintained by a separate team or auto-created by Slurm.

data "slurm_qos" "standard" {
  name = "standard"
}

# A new QOS that inherits the looked-up QOS's wall-clock limit.
resource "slurm_qos" "derived" {
  name        = "derived"
  description = "Reuses the wall-clock limit from the standard QOS"
  priority    = 300
  max_wall_pj = data.slurm_qos.standard.max_wall_pj
}
