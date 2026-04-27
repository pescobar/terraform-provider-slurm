# Terraform / OpenTofu Provider for Slurm

[![Unit Tests](https://github.com/pescobar/terraform-provider-slurm/actions/workflows/unit-tests.yml/badge.svg)](https://github.com/pescobar/terraform-provider-slurm/actions/workflows/unit-tests.yml)
[![Acceptance Tests](https://github.com/pescobar/terraform-provider-slurm/actions/workflows/acceptance-tests.yml/badge.svg)](https://github.com/pescobar/terraform-provider-slurm/actions/workflows/acceptance-tests.yml)

Manages Slurm accounting resources — accounts, users, and QOS — via the
[slurmrestd](https://slurm.schedmd.com/rest.html) REST API.  Covers the same
persistent entities that `sacctmgr` handles; ephemeral objects like jobs and
reservations are out of scope.

> [!WARNING]
> This is a toy project to test Claude Code. Use at your own risk.
> I haven't tested it in a production cluster yet. If I ever test it in production I maybe remove this warning.

## Requirements

| Component | Version |
|-----------|---------|
| OpenTofu  | ≥ 1.6   |
| Terraform | ≥ 1.5   |
| Slurm     | 25.05.x |
| slurmrestd API | v0.0.42 |
| Go (for development) | ≥ 1.22 |

## Authentication

The provider authenticates to slurmrestd using a JWT token passed in the
`X-SLURM-USER-TOKEN` header, either via the `token` argument or the
`SLURM_JWT_TOKEN` environment variable.

### Short-lived token (development)

```bash
export SLURM_JWT_TOKEN=$(scontrol token lifespan=3600 | sed 's/SLURM_JWT=//')
```

The default lifespan is 1800 s (30 min). Fine for one-off runs but not
suitable for production pipelines.

### Long-lived token (production)

`scontrol token` accepts any lifespan in seconds. Generate a token that lasts
a year, store it in your secrets manager or CI secret store, and rotate it
annually:

```bash
# 1 year = 31 536 000 seconds
export SLURM_JWT_TOKEN=$(scontrol token lifespan=31536000 | sed 's/SLURM_JWT=//')
```

Pass it to the provider via environment variable (recommended — keeps it out
of plan output and state):

```bash
export SLURM_JWT_TOKEN="<value from secrets manager>"
tofu apply
```

Or supply it as a sensitive Terraform variable:

```hcl
variable "slurm_token" {
  type      = string
  sensitive = true
}

provider "slurm" {
  endpoint = "http://slurmrestd.example.com:6820"
  token    = var.slurm_token
  cluster  = "mycluster"
}
```

```bash
tofu apply -var="slurm_token=$SLURM_JWT_TOKEN"
```

## Provider Configuration

```hcl
provider "slurm" {
  endpoint    = "http://slurmrestd.example.com:6820"
  token       = var.slurm_token
  cluster     = "mycluster"
  api_version = "v0.0.42"
}
```

All four arguments can also be supplied via environment variables:

| Argument      | Environment variable   | Default   |
|---------------|------------------------|-----------|
| `endpoint`    | `SLURM_REST_URL`       | —         |
| `token`       | `SLURM_JWT_TOKEN`      | —         |
| `cluster`     | `SLURM_CLUSTER`        | —         |
| `api_version` | `SLURM_API_VERSION`    | `v0.0.42` |

## Resources

### `slurm_qos`

Manages a Quality of Service definition.

```hcl
resource "slurm_qos" "standard" {
  name        = "standard"
  description = "Standard priority QOS"
  priority    = 100
  max_wall_pj = 1440   # minutes (24 h)
}

resource "slurm_qos" "priority" {
  name         = "priority"
  description  = "High priority QOS with preemption"
  priority     = 200
  max_wall_pj  = 2880

  preempt_list = [slurm_qos.standard.name]
  preempt_mode = ["CANCEL"]
}
```

> **Note:** Do not manage Slurm's built-in `normal` QOS as a `slurm_qos`
> resource.  Slurm creates it automatically at database init; deleting and
> recreating it via the REST API triggers an internal Slurm bug that causes the
> second `apply` to fail with *"Slurmdbd query returned with empty list"*.

### `slurm_account`

Manages a Slurm account and its cluster-level association (limits, QOS,
fairshare).

```hcl
resource "slurm_account" "physics" {
  name         = "physics"
  description  = "Physics department"
  organization = "university"
  fairshare    = 100
  default_qos  = slurm_qos.standard.name
  allowed_qos  = [slurm_qos.standard.name, slurm_qos.priority.name]
}

resource "slurm_account" "hep" {
  name           = "hep"
  description    = "High Energy Physics group"
  parent_account = slurm_account.physics.name
  fairshare      = 50
}
```

### `slurm_user`

Manages a Slurm user with embedded account associations.  Each `association`
block links the user to one account (optionally scoped to a partition) and
carries per-association limits.

```hcl
resource "slurm_user" "alice" {
  name            = "alice"
  default_account = slurm_account.physics.name

  association {
    account     = slurm_account.physics.name
    fairshare   = 50
    default_qos = slurm_qos.standard.name
    qos         = [slurm_qos.standard.name, slurm_qos.priority.name]
  }

  association {
    account   = slurm_account.hep.name
    fairshare = 20
  }
}
```

## Import

All resources support `import`:

```bash
tofu import slurm_qos.standard     standard
tofu import slurm_account.physics  physics
tofu import slurm_user.alice       alice
```

## Development

### Build and install locally

```bash
make install
```

This builds the binary and places it under
`~/.terraform.d/plugins/registry.terraform.io/pescobar/slurm/0.1.0/`.
Configure a `dev_overrides` block in `~/.tofurc` to use it without a registry
lookup:

```hcl
provider_installation {
  dev_overrides {
    "pescobar/slurm" = "/home/<you>/.terraform.d/plugins/registry.terraform.io/pescobar/slurm/0.1.0/linux_amd64"
  }
  direct {}
}
```

### Run tests

Unit tests (no Slurm cluster required):

```bash
make test
```

Acceptance tests (requires a running slurmrestd — see [Test environment](#test-environment)):

```bash
export SLURM_REST_URL=http://localhost:6820
export SLURM_CLUSTER=linux
export SLURM_JWT_TOKEN=$(docker exec slurmctld scontrol token lifespan=3600 | sed 's/SLURM_JWT=//')
TF_ACC=1 make testacc
```

### Test environment

A self-contained Docker Compose cluster is included in the `docker/` directory
of this repository.  It runs MySQL, slurmdbd, slurmctld, slurmrestd, and a
compute node with port 6820 (slurmrestd) exposed to the host.

```bash
cd docker/
SLURM_VERSION=25.05.4 docker compose up -d
```

Wait for the cluster to be ready:

```bash
docker exec slurmctld scontrol ping
```

Generate a token (valid for 1 hour):

```bash
export TOKEN=$(docker exec slurmctld scontrol token lifespan=3600 \
  | sed 's/SLURM_JWT=//')
```

Run any of the example configs manually:

```bash
# Basic resources
cd examples/
tofu apply  -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"
tofu destroy -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"

# Advanced resources
cd examples/advanced-acceptance-tests/
tofu apply  -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"
tofu destroy -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"

# User association combinations
cd examples/user-association-tests/
tofu apply  -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"
tofu destroy -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"

# Negative tests (expected to fail with a helpful error message)
cd examples/user-association-tests/negative/
tofu apply  -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"
tofu destroy -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"
```

Each directory needs the provider binary in your `PATH` or a `dev_overrides`
block in `~/.tofurc` — see [Build and install locally](#build-and-install-locally).

### Generate documentation

```bash
go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.19.4
tfplugindocs generate --provider-name slurm
```

Rendered docs land in `docs/` and are picked up automatically by the
Terraform and OpenTofu registries.

## License

[MIT](LICENSE)
