---
page_title: "slurm_qos Resource - slurm"
subcategory: ""
description: |-
  Manages a Slurm Quality of Service (QOS) definition.
---

# slurm_qos (Resource)

Manages a Slurm Quality of Service (QOS) definition. Covers the same
parameters that `sacctmgr add qos` / `sacctmgr modify qos` handle:
wall-clock limits, TRES limits, job-count limits, preemption, usage
accounting, and flags.

> **Note:** Do not manage the built-in `normal` QOS with this resource.
> Slurm auto-creates it at database init. Deleting and recreating it via
> the REST API triggers a `Slurmdbd query returned with empty list` error
> on the second apply. You can `import` it, but you should not destroy it.

## Example Usage

### Minimal QOS

```terraform
resource "slurm_qos" "basic" {
  name        = "basic"
  description = "Default priority QOS with no special limits"
  priority    = 100
}
```

### Wall-clock limits

```terraform
resource "slurm_qos" "standard" {
  name        = "standard"
  description = "Standard QOS limited to 24 h per job"
  priority    = 200
  max_wall_pj = 1440 # 24 h in minutes
  grp_wall    = 2880 # 48 h total across all running jobs in this QOS
}
```

### Preemption

```terraform
resource "slurm_qos" "priority" {
  name        = "priority"
  description = "High-priority QOS that preempts standard jobs"
  priority    = 500
  max_wall_pj = 4320 # 72 h

  preempt_list        = [slurm_qos.standard.name]
  preempt_mode        = ["CANCEL"]
  preempt_exempt_time = 300 # jobs safe from preemption for 5 min after start
}
```

### CPU, memory, and GPU TRES limits

TRES (Trackable Resources) blocks use three fields: `type`, `name`, and
`count`. Standard resources (`cpu`, `mem`) leave `name` as `null`. Generic
resources such as GPUs set `type = "gres"` and `name = "gpu"`.

```terraform
resource "slurm_qos" "gpu" {
  name        = "gpu"
  description = "QOS for GPU jobs"
  priority    = 300

  # Per-job limits
  max_tres_per_job = [
    { type = "cpu", count = 128 },
    { type = "mem", count = 512000 }, # MB (≈ 500 GB)
    { type = "gres", name = "gpu", count = 8 },
  ]

  # Per-node limits
  max_tres_per_node = [
    { type = "gres", name = "gpu", count = 4 },
  ]

  # Minimum a job must request to use this QOS
  min_tres_per_job = [
    { type = "gres", name = "gpu", count = 1 },
  ]
}
```

### Group (aggregate) TRES limits

`grp_tres` caps the total resources consumed by *all* running jobs in the
QOS simultaneously. `grp_tres_mins` caps cumulative TRES-minute usage over
the QOS's lifetime (reset by fairshare decay).

```terraform
resource "slurm_qos" "shared_gpu_pool" {
  name        = "shared_gpu_pool"
  description = "GPU pool QOS — caps total GPU usage across all users"
  priority    = 250

  grp_tres = [
    { type = "gres", name = "gpu", count = 32 },
  ]

  grp_tres_mins = [
    { type = "gres", name = "gpu", count = 460800 }, # 32 GPUs × 14400 min (10 d)
  ]
}
```

### Per-user and per-account TRES limits

```terraform
resource "slurm_qos" "fairshare" {
  name        = "fairshare"
  description = "QOS enforcing per-user and per-account TRES caps"
  priority    = 150

  max_tres_per_user = [
    { type = "cpu", count = 256 },
    { type = "gres", name = "gpu", count = 16 },
  ]

  max_tres_mins_per_user = [
    { type = "cpu", count = 2880000 }, # ~2000 CPU·h
    { type = "gres", name = "gpu", count = 57600 }, # 40 GPU·h
  ]

  max_tres_per_account = [
    { type = "gres", name = "gpu", count = 64 },
  ]

  max_tres_mins_per_account = [
    { type = "gres", name = "gpu", count = 230400 }, # 160 GPU·h
  ]
}
```

### Job-count limits

```terraform
resource "slurm_qos" "burst" {
  name        = "burst"
  description = "Burst QOS — caps running and queued job counts"
  priority    = 100

  grp_jobs        = 500  # total running jobs across all users
  grp_submit_jobs = 2000 # total queued jobs across all users

  max_jobs_per_user        = 50
  max_submit_jobs_per_user = 200

  max_jobs_per_account        = 100
  max_submit_jobs_per_account = 400
}
```

### Scavenger (low-priority, no fairshare impact)

```terraform
resource "slurm_qos" "scavenger" {
  name        = "scavenger"
  description = "Low-priority QOS using idle cycles"
  priority    = 10

  usage_factor    = 0 # jobs do not consume fairshare
  usage_threshold = 0 # no minimum usage required to submit

  grace_time  = 120 # 2 min before a preempted job is killed
  max_wall_pj = 360 # 6 h limit

  flags = ["DENY_LIMIT", "NO_DECAY"]
}
```

## Schema

### Required

- `name` (String) The name of the QOS. Changing this forces a new resource.

### Optional

#### Basic settings

| Attribute | sacctmgr name | Description |
|-----------|---------------|-------------|
| `description` | — | Human-readable description. |
| `priority` | `Priority` | Scheduling priority. Higher value = higher priority. |
| `flags` (Set of String) | `Flags` | QOS flags. Must use the REST API spelling (`UPPER_SNAKE_CASE`). Valid values: `PARTITION_MINIMUM_NODE`, `PARTITION_MAXIMUM_NODE`, `PARTITION_TIME_LIMIT`, `ENFORCE_USAGE_THRESHOLD`, `NO_RESERVE`, `REQUIRED_RESERVATION`, `DENY_LIMIT`, `OVERRIDE_PARTITION_QOS`, `NO_DECAY`, `USAGE_FACTOR_SAFE`, `RELATIVE`. |

#### Wall-clock limits

| Attribute | sacctmgr name | Description |
|-----------|---------------|-------------|
| `max_wall_pj` | `MaxWall` | Maximum wall-clock time per job, in minutes. |
| `grp_wall` | `GrpWall` | Maximum total wall-clock minutes usable by all jobs in this QOS at any one time. |

#### Preemption

| Attribute | sacctmgr name | Description |
|-----------|---------------|-------------|
| `preempt_list` (Set of String) | `Preempt` | QOS names that this QOS may preempt. |
| `preempt_mode` (Set of String) | `PreemptMode` | How preemption is enforced: `CANCEL`, `REQUEUE`, `SUSPEND`, `GANG`, `WITHIN`. |
| `preempt_exempt_time` | `PreemptExemptTime` | Seconds a job must run before it becomes eligible for preemption. |

#### Job-count limits

| Attribute | sacctmgr name | Description |
|-----------|---------------|-------------|
| `grp_jobs` | `GrpJobs` | Maximum jobs running simultaneously across all users of this QOS. |
| `grp_submit_jobs` | `GrpSubmit` | Maximum jobs queued (submitted) across all users of this QOS. |
| `max_jobs_per_user` | `MaxJobsPU` | Maximum jobs a single user can run simultaneously. |
| `max_submit_jobs_per_user` | `MaxSubmitPU` | Maximum jobs a single user can have queued. |
| `max_jobs_per_account` | `MaxJobsPA` | Maximum jobs an account can run simultaneously. |
| `max_submit_jobs_per_account` | `MaxSubmitPA` | Maximum jobs an account can have queued. |

#### Usage accounting

| Attribute | sacctmgr name | Description |
|-----------|---------------|-------------|
| `usage_factor` | `UsageFactor` | Multiplier applied to fairshare usage when a job runs under this QOS. `0` means jobs do not affect fairshare at all. Default is `1`. |
| `usage_threshold` | `UsageThres` | Minimum effective usage a user must maintain to submit jobs under this QOS. |
| `grace_time` | `GraceTime` | Seconds before a job that is flagged for preemption is actually killed. |

#### TRES limit attributes

Each TRES attribute is a **set of objects** with three fields:

| Field | Required | Description |
|-------|----------|-------------|
| `type` | yes | TRES type: `cpu`, `mem`, `node`, `energy`, `gres`, etc. |
| `name` | no | Sub-name for generic resources. Required for `gres/gpu`; omit (or set `null`) for `cpu` and `mem`. |
| `count` | yes | Limit value. Memory is in MB. |

| Attribute | sacctmgr name | Description |
|-----------|---------------|-------------|
| `grp_tres` | `GrpTRES` | Maximum TRES usable by all jobs in this QOS simultaneously. |
| `grp_tres_mins` | `GrpTRESMins` | Maximum cumulative TRES-minutes consumable by all jobs in this QOS. |
| `max_tres_per_job` | `MaxTRES` | Maximum TRES a single job can request. |
| `max_tres_mins_per_job` | `MaxTRESMins` | Maximum TRES-minutes a single job can consume. |
| `max_tres_per_node` | `MaxTRESPerNode` | Maximum TRES a single job can use per node. |
| `max_tres_per_user` | `MaxTRESPU` | Maximum TRES a single user can use simultaneously. |
| `max_tres_mins_per_user` | `MaxTRESRunMinsPU` | Maximum cumulative TRES-minutes a single user can consume. |
| `max_tres_per_account` | `MaxTRESPA` | Maximum TRES a single account can use simultaneously. |
| `max_tres_mins_per_account` | `MaxTRESRunMinsPA` | Maximum cumulative TRES-minutes a single account can consume. |
| `min_tres_per_job` | `MinTRES` | Minimum TRES a job must request to be eligible for this QOS. |

All TRES attributes are **Optional + Computed**: you can set them in your
configuration, and after `import` they will be populated from Slurm without
causing plan drift on subsequent applies.

### Read-Only

- `id` (String) The QOS name (same as `name`).

## Import

```bash
tofu import slurm_qos.standard standard
```
