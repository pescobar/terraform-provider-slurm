# Terraform / OpenTofu Provider for Slurm

[![Unit Tests](https://github.com/pescobar/terraform-provider-slurm/actions/workflows/unit-tests.yml/badge.svg)](https://github.com/pescobar/terraform-provider-slurm/actions/workflows/unit-tests.yml)
[![Acceptance Tests](https://github.com/pescobar/terraform-provider-slurm/actions/workflows/acceptance-tests.yml/badge.svg)](https://github.com/pescobar/terraform-provider-slurm/actions/workflows/acceptance-tests.yml)

Manages Slurm HPC accounting resources — accounts, users, and QOS — via the
[slurmrestd](https://slurm.schedmd.com/rest.html) REST API.  Covers the same
persistent entities that `sacctmgr` handles; ephemeral objects like jobs and
reservations are out of scope.

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
`X-SLURM-USER-TOKEN` header.  Tokens are short-lived (30 minutes by default);
generate one with:

```bash
export SLURM_JWT_TOKEN=$(docker exec slurmctld scontrol token lifespan=3600 \
  | sed 's/SLURM_JWT=//')
```

For production deployments consider generating tokens from a JWT key file so
the provider can renew them without manual intervention.

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

The acceptance tests and the example configs were developed against
[`giovtorres/slurm-docker-cluster`](https://github.com/giovtorres/slurm-docker-cluster)
running Slurm 25.05.4.

```bash
git clone https://github.com/giovtorres/slurm-docker-cluster.git
cd slurm-docker-cluster
docker compose up -d
```

Port 6820 (slurmrestd) is exposed to the host.

### Generate documentation

```bash
go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.19.4
tfplugindocs generate --provider-name slurm
```

Rendered docs land in `docs/` and are picked up automatically by the
Terraform and OpenTofu registries.

## License

[MIT](LICENSE)
