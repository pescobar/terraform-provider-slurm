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

  # Load every data/accounts/<stem>.yaml -> { "<stem>" = {…} }. The map key is
  # the filename stem; the real Slurm account name lives in the `name` key.
  accounts = {
    for f in fileset(local.accounts_dir, "*.yaml") :
    trimsuffix(f, ".yaml") => yamldecode(file("${local.accounts_dir}/${f}"))
  }

  # Per-user exceptions only. Everyone else is derived automatically.
  overrides = yamldecode(file("${path.module}/data/users.yaml"))

  # Flatten account membership into normalized association tuples. A member is
  # either a bare string ("alice") or an object with overrides. try() handles
  # both forms and fills unset attributes with null.
  memberships = flatten([
    for acct_key, acct in local.accounts : [
      for m in acct.members : {
        account               = try(acct.name, acct_key)
        user                  = try(m.user, m)
        partition             = try(m.partition, null)
        fairshare             = try(m.fairshare, null)
        priority              = try(m.priority, null)
        default_qos           = try(m.default_qos, null)
        allowed_qos           = try(m.allowed_qos, null)
        max_jobs              = try(m.max_jobs, null)
        max_jobs_accrue       = try(m.max_jobs_accrue, null)
        max_submit_jobs       = try(m.max_submit_jobs, null)
        max_wall_pj           = try(m.max_wall_pj, null)
        grp_jobs              = try(m.grp_jobs, null)
        grp_jobs_accrue       = try(m.grp_jobs_accrue, null)
        grp_submit_jobs       = try(m.grp_submit_jobs, null)
        grp_wall              = try(m.grp_wall, null)
        max_tres_per_job      = try(m.max_tres_per_job, null)
        max_tres_per_node     = try(m.max_tres_per_node, null)
        max_tres_mins_per_job = try(m.max_tres_mins_per_job, null)
        grp_tres              = try(m.grp_tres, null)
        grp_tres_mins         = try(m.grp_tres_mins, null)
        grp_tres_run_mins     = try(m.grp_tres_run_mins, null)
      }
    ]
  ])

  # Invert: group memberships by user into the shape slurm_user needs.
  users = {
    for u in distinct([for m in local.memberships : m.user]) : u => {
      associations   = [for m in local.memberships : m if m.user == u]
      admin_level    = try(local.overrides[u].admin_level, null)
      default_wc_key = try(local.overrides[u].default_wc_key, null)
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

  name           = try(each.value.name, each.key)
  description    = try(each.value.description, null)
  organization   = try(each.value.organization, null)
  parent_account = try(each.value.parent_account, null)
  fairshare      = try(each.value.fairshare, null)
  default_qos    = try(each.value.default_qos, null)
  allowed_qos    = try(each.value.allowed_qos, null)
  max_jobs       = try(each.value.max_jobs, null)

  max_tres_per_job      = try(each.value.max_tres_per_job, null)
  max_tres_per_node     = try(each.value.max_tres_per_node, null)
  max_tres_mins_per_job = try(each.value.max_tres_mins_per_job, null)
  grp_tres              = try(each.value.grp_tres, null)
  grp_tres_mins         = try(each.value.grp_tres_mins, null)
  grp_tres_run_mins     = try(each.value.grp_tres_run_mins, null)

  # QOS referenced by name must exist before the accounts.
  depends_on = [slurm_qos.short, slurm_qos.standard, slurm_qos.long, slurm_qos.high, slurm_qos.low, slurm_qos.gpu, slurm_qos.interactive, slurm_qos.debug]
}

resource "slurm_user" "this" {
  for_each = local.users

  name            = each.key
  default_account = each.value.default_account
  admin_level     = each.value.admin_level
  default_wc_key  = each.value.default_wc_key

  dynamic "association" {
    for_each = each.value.associations
    content {
      account               = association.value.account
      partition             = association.value.partition
      fairshare             = association.value.fairshare
      priority              = association.value.priority
      default_qos           = association.value.default_qos
      allowed_qos           = association.value.allowed_qos
      max_jobs              = association.value.max_jobs
      max_jobs_accrue       = association.value.max_jobs_accrue
      max_submit_jobs       = association.value.max_submit_jobs
      max_wall_pj           = association.value.max_wall_pj
      grp_jobs              = association.value.grp_jobs
      grp_jobs_accrue       = association.value.grp_jobs_accrue
      grp_submit_jobs       = association.value.grp_submit_jobs
      grp_wall              = association.value.grp_wall
      max_tres_per_job      = association.value.max_tres_per_job
      max_tres_per_node     = association.value.max_tres_per_node
      max_tres_mins_per_job = association.value.max_tres_mins_per_job
      grp_tres              = association.value.grp_tres
      grp_tres_mins         = association.value.grp_tres_mins
      grp_tres_run_mins     = association.value.grp_tres_run_mins
    }
  }

  # Accounts (and transitively QOS) must exist before their associations.
  depends_on = [slurm_account.this]
}
