#!/bin/bash
# Adapted from https://github.com/giovtorres/slurm-docker-cluster (MIT License)
# Stripped down to control-plane only: slurmdbd, slurmctld, slurmrestd.
# No compute nodes are needed for testing the Slurm REST API provider.
set -e

echo "---> Starting the MUNGE Authentication service (munged) ..."
mkdir -p /run/munge
chown munge:munge /run/munge
chmod 0755 /run/munge
gosu munge /usr/sbin/munged

if [ "$1" = "slurmdbd" ]; then
    echo "---> Starting the Slurm Database Daemon (slurmdbd) ..."

    # Substitute env vars (MYSQL_USER, MYSQL_PASSWORD) in slurmdbd.conf
    envsubst < /etc/slurm/slurmdbd.conf > /etc/slurm/slurmdbd.conf.tmp
    mv /etc/slurm/slurmdbd.conf.tmp /etc/slurm/slurmdbd.conf
    chown slurm:slurm /etc/slurm/slurmdbd.conf
    chmod 600 /etc/slurm/slurmdbd.conf

    # Generate JWT signing key (shared via etc_slurm volume with slurmctld/slurmrestd)
    if [ ! -f /etc/slurm/jwt_hs256.key ]; then
        dd if=/dev/random of=/etc/slurm/jwt_hs256.key bs=32 count=1
        chown slurm:slurm /etc/slurm/jwt_hs256.key
        chmod 0600 /etc/slurm/jwt_hs256.key
    fi

    until echo "SELECT 1" | mysql -h mysql -u${MYSQL_USER} -p${MYSQL_PASSWORD} 2>&1 >/dev/null; do
        echo "-- Waiting for MySQL ..."
        sleep 2
    done
    echo "-- MySQL is ready."

    exec gosu slurm /usr/sbin/slurmdbd -Dvvv
fi

if [ "$1" = "slurmctld" ]; then
    echo "---> Waiting for slurmdbd ..."
    until 2>/dev/null >/dev/tcp/slurmdbd/6819; do
        echo "-- slurmdbd not yet available, sleeping ..."
        sleep 2
    done
    echo "-- slurmdbd is ready."

    echo "---> Starting the Slurm Controller Daemon (slurmctld) ..."
    exec gosu slurm /usr/sbin/slurmctld -i -Dvvv
fi

if [ "$1" = "slurmrestd" ]; then
    echo "---> Waiting for slurmctld ..."
    until 2>/dev/null >/dev/tcp/slurmctld/6817; do
        echo "-- slurmctld not yet available, sleeping ..."
        sleep 2
    done
    echo "-- slurmctld is ready."

    echo "---> Starting the Slurm REST API Daemon (slurmrestd) ..."
    mkdir -p /var/run/slurmrestd
    chown slurmrest:slurmrest /var/run/slurmrestd

    export SLURM_JWT=daemon
    exec gosu slurmrest /usr/sbin/slurmrestd -vvv \
        unix:/var/run/slurmrestd/slurmrestd.socket 0.0.0.0:6820
fi

exec "$@"
