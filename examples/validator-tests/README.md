# validator-tests

Negative acceptance fixture covering the plan-time schema validators wired
into `slurm_account`, `slurm_user`, and `slurm_qos`. Every resource block in
`main.tf` deliberately violates a validator, so this fixture is **expected to
fail at `tofu plan`** with one diagnostic per violation. No API calls are
made and no Slurm state is mutated — there is nothing to apply or destroy.

## Run

```bash
cd examples/validator-tests
TOKEN=$(docker exec slurmctld scontrol token lifespan=600 | sed 's/SLURM_JWT=//')
tofu plan -var "slurm_token=$TOKEN"
```

## What's covered

| Resource | Attribute | Validator class |
|---|---|---|
| `slurm_account.neg_fairshare` | `fairshare = -5` | `AtLeast(0)` |
| `slurm_account.neg_max_jobs` | `max_jobs = -1` | `AtLeast(0)` |
| `slurm_user.neg_admin_level` | `admin_level = "Sudo"` | `OneOf("None","Operator","Administrator")` |
| `slurm_user.neg_assoc_max_jobs` | `association { max_jobs = -10 }` | `AtLeast(0)` (nested in block) |
| `slurm_qos.neg_flags` | `flags = ["MADE_UP_FLAG"]` | `OneOf(qosFlagValues...)` |
| `slurm_qos.neg_preempt_mode` | `preempt_mode = ["panic"]` | `OneOf(qosPreemptModeValues...)` |
| `slurm_qos.neg_priority` | `priority = -1` | `AtLeast(0)` |
| `slurm_qos.neg_tres_count` | `grp_tres = [{count = -100}]` | `AtLeast(0)` (TRES count) |

## Pass criteria

`tofu plan` exits non-zero and prints **eight** "Invalid Attribute Value" /
"Invalid Attribute Value Match" diagnostics — one per resource. If a
validator regresses (e.g. is removed from a schema attribute), one of the
violations would slip through plan and either fail later as an opaque API
error or apply silently with bad data — both regressions this fixture is
designed to catch.
