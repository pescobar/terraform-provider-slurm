# Look up a partition defined in slurm.conf and reference it from an
# association, with a plan-time guarantee that the partition exists.
data "slurm_partition" "main" {
  name = "cpu"
}

resource "slurm_user" "dave" {
  name            = "dave"
  default_account = "physics"

  association {
    account   = "physics"
    partition = data.slurm_partition.main.name
    max_jobs  = 20
  }
}
