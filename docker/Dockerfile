# Multi-stage Dockerfile for a minimal Slurm test cluster on Ubuntu 24.04.
# Compiles Slurm from the official SchedMD source tarball.
# Services (slurmdbd / slurmctld / slurmrestd) are selected via CMD.

ARG SLURM_VERSION
ARG GOSU_VERSION=1.19

# ============================================================================
# Stage 1: Build gosu from source (avoids CVEs in pre-built binaries)
# https://github.com/tianon/gosu/issues/136
# ============================================================================
FROM golang:1.24-bookworm AS gosu-builder

ARG GOSU_VERSION
ARG TARGETOS
ARG TARGETARCH

RUN git clone --branch ${GOSU_VERSION} --depth 1 \
      https://github.com/tianon/gosu.git /go/src/github.com/tianon/gosu \
    && cd /go/src/github.com/tianon/gosu \
    && go mod download \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
       go build -v -trimpath -ldflags '-d -w' -o /go/bin/gosu . \
    && chmod +x /go/bin/gosu

# ============================================================================
# Stage 2: Compile Slurm and install to /build (used as DESTDIR)
# ============================================================================
FROM ubuntu:24.04 AS builder

ARG SLURM_VERSION
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    wget \
    bzip2 \
    pkg-config \
    libmunge-dev \
    libssl-dev \
    libmariadbd-dev \
    libhwloc-dev \
    libjwt-dev \
    libhttp-parser-dev \
    libjson-c-dev \
    libyaml-dev \
    liblua5.3-dev \
    liblz4-dev \
    libreadline-dev \
    libpam0g-dev \
    libnuma-dev \
    python3 \
    && rm -rf /var/lib/apt/lists/*

RUN wget --no-check-certificate -q -O /tmp/slurm-${SLURM_VERSION}.tar.bz2 \
      https://download.schedmd.com/slurm/slurm-${SLURM_VERSION}.tar.bz2 \
    && tar xf /tmp/slurm-${SLURM_VERSION}.tar.bz2 -C /tmp \
    && cd /tmp/slurm-${SLURM_VERSION} \
    && ./configure \
         --prefix=/usr \
         --sysconfdir=/etc/slurm \
         --localstatedir=/var \
         --with-munge \
         --with-jwt \
         --with-mysql_config \
         --with-slurmrestd \
         --without-ofed \
    && make -j$(nproc) \
    && make install DESTDIR=/build \
    && rm -rf /tmp/slurm-*

# ============================================================================
# Stage 3: Minimal runtime image
# ============================================================================
FROM ubuntu:24.04

ARG SLURM_VERSION
ARG DEBIAN_FRONTEND=noninteractive

LABEL org.opencontainers.image.source="https://github.com/pescobar/terraform-provider-slurm" \
      org.opencontainers.image.title="slurm-test" \
      org.opencontainers.image.description="Slurm ${SLURM_VERSION} test cluster (Ubuntu 24.04) — no compute nodes"

RUN apt-get update && apt-get install -y --no-install-recommends \
    munge \
    libssl3 \
    mariadb-client \
    libhwloc15 \
    libjwt2 \
    libhttp-parser2.9 \
    libjson-c5 \
    libyaml-0-2 \
    liblua5.3-0 \
    liblz4-1 \
    libreadline8 \
    libpam0g \
    libnuma1 \
    gettext-base \
    curl \
    jq \
    && rm -rf /var/lib/apt/lists/*

COPY --from=gosu-builder /go/bin/gosu /usr/local/bin/gosu
COPY --from=builder /build /

RUN groupadd -r --gid=990 slurm \
    && useradd -r -g slurm --uid=990 slurm \
    && groupadd -r --gid=991 slurmrest \
    && useradd -r -g slurmrest --uid=991 slurmrest \
    && dd if=/dev/urandom of=/etc/munge/munge.key bs=1 count=1024 \
    && chmod 400 /etc/munge/munge.key \
    && chown munge:munge /etc/munge/munge.key \
    && mkdir -m 0755 -p \
         /var/run/slurm \
         /var/spool/slurm \
         /var/lib/slurm \
         /var/log/slurm \
         /etc/slurm \
    && chown slurm:slurm \
         /var/run/slurm \
         /var/spool/slurm \
         /var/lib/slurm \
         /var/log/slurm \
         /etc/slurm

COPY config/ /tmp/slurm-config/
RUN set -e \
    && MAJOR_MINOR=$(echo ${SLURM_VERSION} | cut -d. -f1,2) \
    && if [ -f "/tmp/slurm-config/${MAJOR_MINOR}/slurm.conf" ]; then \
         cp /tmp/slurm-config/${MAJOR_MINOR}/slurm.conf /etc/slurm/slurm.conf; \
       else \
         echo "No version-specific slurm.conf for ${MAJOR_MINOR}, using 25.11" >&2; \
         cp /tmp/slurm-config/25.11/slurm.conf /etc/slurm/slurm.conf; \
       fi \
    && cp /tmp/slurm-config/common/slurmdbd.conf /etc/slurm/slurmdbd.conf \
    && cp /tmp/slurm-config/common/cgroup.conf /etc/slurm/cgroup.conf \
    && cp /tmp/slurm-config/common/gres.conf /etc/slurm/gres.conf \
    && chown slurm:slurm /etc/slurm/slurm.conf /etc/slurm/slurmdbd.conf /etc/slurm/cgroup.conf /etc/slurm/gres.conf \
    && chmod 644 /etc/slurm/slurm.conf /etc/slurm/cgroup.conf /etc/slurm/gres.conf \
    && chmod 600 /etc/slurm/slurmdbd.conf \
    && rm -rf /tmp/slurm-config

COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["slurmdbd"]
