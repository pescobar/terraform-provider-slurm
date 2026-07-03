# ---------------------------------------------------------------------------
# generate.tf — write once, rarely touched.
#
# Sysadmins edit the human-friendly, ACCOUNT-CENTRIC data under data/:
#   - data/accounts/<name>.yaml : account metadata + its member list
#   - data/users.yaml           : ONLY exceptions (admins, multi-account default)
#
# This file inverts that account-centric data into the USER-CENTRIC resources
# the Slurm provider requires (a slurm_user carries all of its associations).
# ---------------------------------------------------------------------------

locals {
  accounts_dir = "${path.module}/data/accounts"

  # Load every data/accounts/<name>.yaml -> { "<name>" = {…} }
  accounts = {
    for f in fileset(local.accounts_dir, "*.yaml") :
    trimsuffix(f, ".yaml") => yamldecode(file("${local.accounts_dir}/${f}"))
  }

  # Per-user exceptions only. Everyone else is derived automatically.
  overrides = yamldecode(file("${path.module}/data/users.yaml"))

  # Flatten account membership into normalized (user, account, …overrides)
  # tuples. A member is either a bare string ("alice") or an object with
  # overrides ({ user = "alice", qos = [...] }). try() handles both forms:
  # for a string, m.user / m.default_qos error out and fall back.
  memberships = flatten([
    for acct_name, acct in local.accounts : [
      for m in acct.members : {
        account     = acct_name
        user        = try(m.user, m) # object -> m.user, string -> m itself
        default_qos = try(m.default_qos, null)
        qos         = try(m.qos, null)
        fairshare   = try(m.fairshare, null)
        max_jobs    = try(m.max_jobs, null)
        partition   = try(m.partition, null)
      }
    ]
  ])

  # Invert: group memberships by user into the shape slurm_user needs.
  users = {
    for u in distinct([for m in local.memberships : m.user]) : u => {
      associations = [for m in local.memberships : m if m.user == u]
      admin_level  = try(local.overrides[u].admin_level, null)
      # Login default account: explicit override, else the user's (only) account.
      default_account = try(
        local.overrides[u].default_account,
        [for m in local.memberships : m.account if m.user == u][0],
      )
    }
  }
}

resource "slurm_account" "this" {
  for_each = local.accounts

  name           = each.key
  description    = try(each.value.description, each.key)
  organization   = try(each.value.organization, each.key)
  parent_account = try(each.value.parent_account, null)
  fairshare      = try(each.value.fairshare, null)
  default_qos    = try(each.value.default_qos, null)
  allowed_qos    = try(each.value.allowed_qos, null)

  # QOS referenced by name must exist first.
  depends_on = [
    slurm_qos.short, slurm_qos.standard, slurm_qos.long, slurm_qos.high,
    slurm_qos.low, slurm_qos.gpu, slurm_qos.interactive, slurm_qos.debug,
  ]
}

resource "slurm_user" "this" {
  for_each = local.users

  name            = each.key
  default_account = each.value.default_account
  admin_level     = each.value.admin_level

  dynamic "association" {
    for_each = each.value.associations
    content {
      account     = association.value.account
      default_qos = try(association.value.default_qos, null)
      qos         = try(association.value.qos, null)
      fairshare   = try(association.value.fairshare, null)
      max_jobs    = try(association.value.max_jobs, null)
      partition   = try(association.value.partition, null)
    }
  }

  # Accounts (and transitively QOS) must exist before their associations.
  depends_on = [slurm_account.this]
}
