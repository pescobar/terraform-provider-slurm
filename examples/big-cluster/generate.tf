# ---------------------------------------------------------------------------
# generate.tf — write once, rarely touched.
#
# Sysadmins edit the human-friendly, ACCOUNT-CENTRIC data under data/:
#   - data/accounts/<name>.yaml : account metadata + its user_associations list
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

  # Flatten each account's user_associations into normalized association
  # tuples. An entry is either a bare string ("alice", no overrides) or an
  # object with two independent sub-maps -- try() handles both forms and
  # fills unset attributes with null:
  #
  #   account_overrides -- fields slurm_account also has (fairshare,
  #     default_qos, allowed_qos, max_jobs, TRES limits). Most of these
  #     inherit the account's value when omitted here (default_qos, max_jobs,
  #     max_tres_per_job, max_tres_per_node, max_tres_mins_per_job,
  #     allowed_qos); fairshare and grp_tres/grp_tres_mins/grp_tres_run_mins
  #     do NOT inherit -- Slurm falls back to its own fixed default instead.
  #     See "What an omitted account_overrides key resolves to" in README.md.
  #
  #   association -- fields that exist ONLY at the association level
  #     (partition, priority, job-count and wall-clock limits). There is no
  #     account-level equivalent to inherit from; these are declared here,
  #     not overridden.
  #
  # See "Account-level fields vs association-only fields" in README.md.
  memberships = flatten([
    for acct_key, acct in local.accounts : [
      for m in acct.user_associations : {
        account = try(acct.name, acct_key)
        user    = try(m.user, m)
        # account_overrides.*
        # fairshare is a string ("parent" or an integer weight); tostring()
        # accepts either a quoted or a bare-number value in the YAML.
        fairshare             = try(tostring(m.account_overrides.fairshare), null)
        default_qos           = try(m.account_overrides.default_qos, null)
        allowed_qos           = try(m.account_overrides.allowed_qos, null)
        max_jobs              = try(m.account_overrides.max_jobs, null)
        max_tres_per_job      = try(m.account_overrides.max_tres_per_job, null)
        max_tres_per_node     = try(m.account_overrides.max_tres_per_node, null)
        max_tres_mins_per_job = try(m.account_overrides.max_tres_mins_per_job, null)
        grp_tres              = try(m.account_overrides.grp_tres, null)
        grp_tres_mins         = try(m.account_overrides.grp_tres_mins, null)
        grp_tres_run_mins     = try(m.account_overrides.grp_tres_run_mins, null)
        # association.*
        partition       = try(m.association.partition, null)
        priority        = try(m.association.priority, null)
        max_jobs_accrue = try(m.association.max_jobs_accrue, null)
        max_submit_jobs = try(m.association.max_submit_jobs, null)
        max_wall_pj     = try(m.association.max_wall_pj, null)
        grp_jobs        = try(m.association.grp_jobs, null)
        grp_jobs_accrue = try(m.association.grp_jobs_accrue, null)
        grp_submit_jobs = try(m.association.grp_submit_jobs, null)
        grp_wall        = try(m.association.grp_wall, null)
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
  fairshare      = try(tostring(each.value.fairshare), null)
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
