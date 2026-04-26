# Examples

This directory contains a working example that creates two QOS, one account,
and one user with an association using the Slurm provider.

## Prerequisites

- [OpenTofu](https://opentofu.org/) installed
- A running Slurm cluster with slurmrestd exposed on port 6820
  (see `../docker/` for a local test cluster)
- A `~/.tofurc` with dev_overrides pointing to the locally built provider binary
  (only needed when testing a local build, not a released version)

## Start the local test cluster

```bash
cd ../docker
SLURM_VERSION=25.05.4 docker compose up -d
```

## Generate a JWT token

```bash
TOKEN=$(docker exec slurmctld scontrol token lifespan=3600 | sed 's/SLURM_JWT=//')
```

## Run the example

```bash
cd examples/
tofu init
tofu apply -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"
```

## Destroy

```bash
tofu destroy -var="slurm_token=$TOKEN" -var="slurm_api_version=v0.0.42"
```
