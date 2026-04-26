resource "slurm_qos" "standard" {
  name        = "standard"
  description = "Standard priority QOS"
  priority    = 100
  max_wall_pj = 1440
}

resource "slurm_qos" "priority" {
  name        = "priority"
  description = "High priority QOS with preemption"
  priority    = 200
  max_wall_pj = 2880

  preempt_list = [slurm_qos.standard.name]
  preempt_mode = ["CANCEL"]
}
