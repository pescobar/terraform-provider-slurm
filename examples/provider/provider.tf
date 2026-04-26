provider "slurm" {
  endpoint    = "http://slurmrestd.example.com:6820"
  token       = var.slurm_token
  cluster     = "mycluster"
  api_version = "v0.0.42"
}

variable "slurm_token" {
  type      = string
  sensitive = true
}
