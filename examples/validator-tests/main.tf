// validator-tests/main.tf
//
// Negative acceptance fixture for plan-time schema validators.
// Every resource in this file deliberately violates a validator. A successful
// run is one where `tofu plan` exits non-zero with a diagnostic per
// violation — no API calls are made and no Slurm state is mutated.
//
// Run:
//   cd examples/validator-tests
//   tofu plan -var "slurm_token=$SLURM_JWT_TOKEN"
//
// Expected: 8 plan-time errors (one per resource block below), each citing
// the offending attribute path. Exit code != 0.

terraform {
  required_providers {
    slurm = {
      source = "pescobar/slurm"
    }
  }
}

provider "slurm" {
  endpoint    = "http://localhost:6820"
  token       = var.slurm_token
  cluster     = "linux"
  api_version = var.slurm_api_version
}

variable "slurm_token" {
  type      = string
  sensitive = true
}

variable "slurm_api_version" {
  type    = string
  default = "v0.0.42"
}

# ============================================================================
# slurm_account — AtLeast(0) on fairshare and max_jobs
# ============================================================================

# Negative fairshare — expect "value must be at least 0".
resource "slurm_account" "neg_fairshare" {
  name      = "neg_fairshare"
  fairshare = -5
}

# Negative max_jobs — expect "value must be at least 0".
resource "slurm_account" "neg_max_jobs" {
  name     = "neg_max_jobs"
  max_jobs = -1
}

# ============================================================================
# slurm_user — admin_level OneOf and AtLeast(0) on association limits
# ============================================================================

# Unknown admin_level — expect "value must be one of: [None Operator
# Administrator]".
resource "slurm_user" "neg_admin_level" {
  name            = "neg_admin"
  default_account = "neg_acct"
  admin_level     = "Sudo"

  association {
    account = "neg_acct"
  }
}

# Negative max_jobs nested in an association block — expect the validator
# error to include the association path.
resource "slurm_user" "neg_assoc_max_jobs" {
  name            = "neg_assoc"
  default_account = "neg_acct"

  association {
    account  = "neg_acct"
    max_jobs = -10
  }
}

# ============================================================================
# slurm_qos — flags OneOf, preempt_mode OneOf, AtLeast(0) on numeric, TRES
# count AtLeast(0)
# ============================================================================

# Invalid flag value — expect "value must be one of: [PARTITION_MINIMUM_NODE
# … RELATIVE]".
resource "slurm_qos" "neg_flags" {
  name  = "neg_flags"
  flags = ["MADE_UP_FLAG"]
}

# Invalid preempt_mode — expect "value must be one of: [OFF CANCEL GANG
# REQUEUE SUSPEND WITHIN]".
resource "slurm_qos" "neg_preempt_mode" {
  name         = "neg_preempt_mode"
  preempt_mode = ["panic"]
}

# Negative priority — expect AtLeast(0) error.
resource "slurm_qos" "neg_priority" {
  name     = "neg_priority"
  priority = -1
}

# Negative TRES count — expect AtLeast(0) error from the shared TRES schema.
resource "slurm_qos" "neg_tres_count" {
  name = "neg_tres_count"
  grp_tres = [
    { type = "cpu", count = -100 },
  ]
}
