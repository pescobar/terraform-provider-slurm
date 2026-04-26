#!/usr/bin/env bash
# reset_slurmdbd.sh — Delete all accounting resources from slurmdbd.
# Works regardless of how resources were created (tofu, sacctmgr, REST API).
#
# Usage:
#   ./test/reset_slurmdbd.sh [container_name]
#
# Defaults to container name "slurmctld". Override with:
#   SLURM_CONTAINER=mycontainer ./test/reset_slurmdbd.sh

set -euo pipefail

CONTAINER="${1:-${SLURM_CONTAINER:-slurmctld}}"

sacctmgr() {
    docker exec "$CONTAINER" sacctmgr "$@"
}

section() { echo; echo "=== $* ==="; }
deleted() { echo "  deleted: $*"; }
skipped() { echo "  skipped: $*"; }
none()    { echo "  (none)"; }

# ---------------------------------------------------------------------------
# 1. Users — delete first so associations are cleared before accounts go away
# ---------------------------------------------------------------------------
section "Users"
mapfile -t USERS < <(sacctmgr show user -P -n format=User 2>/dev/null | grep -v '^$' || true)
if [[ ${#USERS[@]} -eq 0 ]]; then
    none
else
    for user in "${USERS[@]}"; do
        if sacctmgr -i delete user name="$user" 2>/dev/null | grep -q "Deleting"; then
            deleted "$user"
        else
            skipped "$user (already gone or protected)"
        fi
    done
fi

# ---------------------------------------------------------------------------
# 2. Accounts — skip root (it cannot be deleted)
# ---------------------------------------------------------------------------
section "Accounts"
mapfile -t ACCOUNTS < <(sacctmgr show account -P -n format=Account 2>/dev/null | grep -v '^root$' | grep -v '^$' || true)
if [[ ${#ACCOUNTS[@]} -eq 0 ]]; then
    none
else
    for account in "${ACCOUNTS[@]}"; do
        if sacctmgr -i delete account name="$account" 2>/dev/null | grep -q "Deleting"; then
            deleted "$account"
        else
            skipped "$account (already gone or has dependents)"
        fi
    done
fi

# ---------------------------------------------------------------------------
# 3. QOS — safe to delete after accounts are gone
# ---------------------------------------------------------------------------
section "QOS"
mapfile -t QOS_LIST < <(sacctmgr show qos -P -n format=Name 2>/dev/null | grep -v '^$' || true)
if [[ ${#QOS_LIST[@]} -eq 0 ]]; then
    none
else
    for qos in "${QOS_LIST[@]}"; do
        if sacctmgr -i delete qos name="$qos" 2>/dev/null | grep -q "Deleting"; then
            deleted "$qos"
        else
            skipped "$qos (already gone or in use)"
        fi
    done
fi

# ---------------------------------------------------------------------------
# 4. Clusters — optional, commented out by default.
# Deleting the cluster that slurmctld is registered to will break the daemon.
# Uncomment only if you intend to re-register the cluster afterwards.
# ---------------------------------------------------------------------------
# section "Clusters"
# mapfile -t CLUSTERS < <(sacctmgr show cluster -P -n format=Cluster 2>/dev/null | grep -v '^$' || true)
# if [[ ${#CLUSTERS[@]} -eq 0 ]]; then
#     none
# else
#     for cluster in "${CLUSTERS[@]}"; do
#         if sacctmgr -i delete cluster name="$cluster" 2>/dev/null | grep -q "Deleting"; then
#             deleted "$cluster"
#         else
#             skipped "$cluster"
#         fi
#     done
# fi

echo
echo "Done. Slurmdbd reset complete."
