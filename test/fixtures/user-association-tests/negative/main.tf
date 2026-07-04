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
# Infrastructure — valid QOS and accounts used to construct violations below.
# ============================================================================

resource "slurm_qos" "neg_low" {
  name = "neg_low"
}

resource "slurm_qos" "neg_medium" {
  name = "neg_medium"
}

resource "slurm_qos" "neg_high" {
  name = "neg_high"
}

# Top-level account with explicit default_qos and allowed_qos.
resource "slurm_account" "neg_dept" {
  name         = "neg_dept"
  organization = "neg_org"
  default_qos  = slurm_qos.neg_medium.name
  allowed_qos  = [slurm_qos.neg_low.name, slurm_qos.neg_medium.name, slurm_qos.neg_high.name]
}

# Child account with NO direct allowed_qos (inherits from neg_dept).
resource "slurm_account" "neg_child" {
  name           = "neg_child"
  parent_account = slurm_account.neg_dept.name
  organization   = "neg_org"
}

# ============================================================================
# Rule 1 violation — qos list blocks the account's default_qos
#
# neg_dept has default_qos=neg_medium. The association's qos list contains
# only [neg_low, neg_high], so neg_medium is excluded. Slurm rejects this
# because the user's effective default QOS is not in the allowed list.
#
# Expected provider error:
#   "Slurm enforces two QOS access rules … Rule 1 — qos list overrides …"
# ============================================================================
resource "slurm_user" "rule1_violation" {
  name            = "neg_u_rule1"
  default_account = slurm_account.neg_dept.name

  association {
    account = slurm_account.neg_dept.name
    qos     = [slurm_qos.neg_low.name, slurm_qos.neg_high.name]
  }
}

# ============================================================================
# Rule 2 violation — default_qos not in the account's direct allowed_qos
#
# neg_child has no direct allowed_qos of its own. Slurm does NOT walk up to
# the parent account to check inherited QOS, so setting default_qos=neg_low
# on this association fails even though neg_low IS in neg_dept's allowed_qos.
#
# Expected provider error:
#   "Slurm enforces two QOS access rules … Rule 2 — default_qos requires …"
# ============================================================================
resource "slurm_user" "rule2_violation" {
  name            = "neg_u_rule2"
  default_account = slurm_account.neg_child.name

  association {
    account     = slurm_account.neg_child.name
    default_qos = slurm_qos.neg_low.name
  }
}
