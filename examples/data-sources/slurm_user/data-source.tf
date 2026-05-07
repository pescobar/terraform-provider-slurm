# Look up an existing Slurm user by name. Returns the user's admin level,
# default account, and every association with its full limit set as a
# nested block — exactly the shape slurm_user accepts on input.

data "slurm_user" "alice" {
  name = "alice"
}

# Use the data source's default_account in another resource. Useful in
# multi-team setups where the user is managed elsewhere but the account
# binding needs to be referenced.
output "alice_default_account" {
  value = data.slurm_user.alice.default_account
}

# All accounts the user has an association with.
output "alice_accounts" {
  value = [for a in data.slurm_user.alice.association : a.account]
}
