# Slurm Test Cluster Docker Images

Pre-built images are published to the GitHub Container Registry (GHCR) at
`ghcr.io/pescobar/slurm-test:<slurm-version>` and used by the provider's
acceptance tests. Images are built and pushed manually when a new Slurm
version needs to be tested.

## Slurm version → REST API version

Each Slurm release ships a specific set of slurmrestd API versions. Use the
newest one the release advertises. The values below are what the CI matrix
and the `SLURM_API_VERSION` provider setting expect:

| Slurm version | REST API version |
|---------------|------------------|
| 25.05.x       | v0.0.42          |
| 25.11.x       | v0.0.44          |
| 26.05.x       | v0.0.45          |

## Building a new image

Run from inside the `docker/` directory:

```bash
docker build --build-arg SLURM_VERSION=26.05.1 -t ghcr.io/pescobar/slurm-test:26.05.1 .
```

The build compiles Slurm from the official SchedMD source tarball — expect
15-20 minutes on first build. Subsequent builds for the same base OS layer
are faster due to Docker layer caching.

## Verifying the image locally

Copy the example env file and set the version you just built:

```bash
cp .env.example .env
# edit .env and set SLURM_VERSION=26.05.1
```

Start the cluster:

```bash
SLURM_VERSION=26.05.1 docker compose up -d
```

Watch the logs until all services are healthy:

```bash
docker compose logs -f
```

Verify slurmctld is up:

```bash
docker exec slurmctld scontrol ping
```

Verify slurmrestd is responding. Use the API version that matches the Slurm
version you built (see the mapping table below):

```bash
TOKEN=$(docker exec slurmctld scontrol token lifespan=600 | sed 's/SLURM_JWT=//')
curl -sf -H "X-SLURM-USER-TOKEN: $TOKEN" http://localhost:6820/slurmdb/v0.0.45/diag/ | jq .
```

The advertised versions can be listed with:

```bash
curl -s -H "X-SLURM-USER-TOKEN: $TOKEN" http://localhost:6820/openapi/v3 \
  | jq '.paths | keys | map(select(startswith("/slurmdb"))) | map(split("/")[2]) | unique'
```

Tear down when done:

```bash
docker compose down -v
```

## Pushing the image to GHCR

Log in to the GitHub Container Registry using a Personal Access Token (PAT)
with `write:packages` scope:

```bash
docker login ghcr.io -u pescobar
# enter your PAT when prompted
```

Or non-interactively if the PAT is stored in an environment variable:

```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u pescobar --password-stdin
```

Push the image:

```bash
docker push ghcr.io/pescobar/slurm-test:26.05.1
```

The image is public — no authentication is required to pull it.

## Adding the new version to CI

Once the image is pushed, add a new entry to the matrix in
`.github/workflows/acceptance-tests.yml`:

```yaml
matrix:
  include:
    - slurm_version: "25.05.4"
      api_version: "v0.0.42"
    - slurm_version: "25.11.5"
      api_version: "v0.0.44"
    - slurm_version: "26.05.1"
      api_version: "v0.0.45"
```
