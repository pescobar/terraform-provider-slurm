# Slurm Test Cluster Docker Images

Pre-built images are published to the GitHub Container Registry (GHCR) at
`ghcr.io/pescobar/slurm-test:<slurm-version>` and used by the provider's
acceptance tests. Images are built and pushed manually when a new Slurm
version needs to be tested.

## Building a new image

Run from inside the `docker/` directory:

```bash
docker build --build-arg SLURM_VERSION=25.11.5 -t ghcr.io/pescobar/slurm-test:25.11.5 .
```

The build compiles Slurm from the official SchedMD source tarball — expect
15-20 minutes on first build. Subsequent builds for the same base OS layer
are faster due to Docker layer caching.

## Verifying the image locally

Copy the example env file and set the version you just built:

```bash
cp .env.example .env
# edit .env and set SLURM_VERSION=25.11.5
```

Start the cluster:

```bash
SLURM_VERSION=25.11.5 docker compose up -d
```

Watch the logs until all services are healthy:

```bash
docker compose logs -f
```

Verify slurmctld is up:

```bash
docker exec slurmctld scontrol ping
```

Verify slurmrestd is responding (adjust the API version as needed):

```bash
TOKEN=$(docker exec slurmctld scontrol token lifespan=600 | sed 's/SLURM_JWT=//')
curl -sf -H "X-SLURM-USER-TOKEN: $TOKEN" http://localhost:6820/slurmdb/v0.0.44/diag/ | jq .
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
docker push ghcr.io/pescobar/slurm-test:25.11.5
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
```
