# Generic cluster-wide QOS, kept as plain HCL — there are only a handful and
# they change rarely. Names here are arbitrary examples; never manage Slurm's
# built-in "normal" QOS (see CLAUDE.md).

resource "slurm_qos" "short" {
  name        = "short"
  description = "Short jobs, fast turnaround"
  priority    = 150
  max_wall_pj = 60
}

resource "slurm_qos" "standard" {
  name        = "standard"
  description = "Default general-purpose QOS"
  priority    = 100
  max_wall_pj = 1440
}

resource "slurm_qos" "long" {
  name        = "long"
  description = "Long-running jobs"
  priority    = 80
  max_wall_pj = 10080
}

resource "slurm_qos" "high" {
  name        = "high"
  description = "High priority"
  priority    = 300
  max_wall_pj = 2880
}

resource "slurm_qos" "low" {
  name        = "low"
  description = "Low priority / backfill"
  priority    = 10
  max_wall_pj = 4320
}

resource "slurm_qos" "gpu" {
  name        = "gpu"
  description = "GPU partitions"
  priority    = 200
  max_wall_pj = 2880
}

resource "slurm_qos" "interactive" {
  name        = "interactive"
  description = "Interactive sessions"
  priority    = 250
  max_wall_pj = 480
}

resource "slurm_qos" "debug" {
  name        = "debug"
  description = "Debugging, short limits"
  priority    = 250
  max_wall_pj = 30
}
