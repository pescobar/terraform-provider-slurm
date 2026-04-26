#!/usr/bin/env bash
# populate_import_test_data.sh — Create diverse Slurm entities for import testing.
#
# Creates QOS, accounts, and users with various attribute combinations so that
# every import code path is exercised. Uses the REST API directly (same path
# as the provider) so field names and formats are guaranteed to match.
#
# All test resources are prefixed with "tacc-" to distinguish them from
# production data and to make cleanup easy.
#
# Prerequisites:
#   - slurmrestd reachable at SLURM_REST_URL (default: http://localhost:6820)
#   - SLURM_JWT_TOKEN set to a valid JWT
#   - jq installed
#
# Usage:
#   SLURM_JWT_TOKEN=$(docker exec slurmctld scontrol token lifespan=3600 | awk -F= '{print $2}') \
#     ./test/populate_import_test_data.sh

set -euo pipefail

URL="${SLURM_REST_URL:-http://localhost:6820}"
API="${SLURM_API_VERSION:-v0.0.42}"
CLUSTER="${SLURM_CLUSTER:-linux}"
CONTAINER="${SLURM_CONTAINER:-slurmctld}"

# Resolve the JWT token. Priority order:
#   1. SLURM_JWT_TOKEN (provider convention)
#   2. SLURM_JWT       (Slurm's own env var, set by "eval $(scontrol token ...)")
#   3. Auto-generate via docker exec if the slurmctld container is running
# Note: use sed to strip the "SLURM_JWT=" prefix so base64 padding chars (=)
# in the token itself are preserved — awk -F= loses them.
TOKEN="${SLURM_JWT_TOKEN:-${SLURM_JWT:-}}"
if [[ -z "$TOKEN" ]]; then
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${CONTAINER}$"; then
        TOKEN=$(docker exec "$CONTAINER" scontrol token lifespan=3600 2>/dev/null \
                | sed 's/^SLURM_JWT=//')
    fi
fi
if [[ -z "$TOKEN" ]]; then
    echo "Error: could not obtain a JWT token." >&2
    echo "  Set SLURM_JWT_TOKEN, or run:" >&2
    echo "    export SLURM_JWT_TOKEN=\$(docker exec slurmctld scontrol token lifespan=3600 | sed 's/SLURM_JWT=//')" >&2
    exit 1
fi

api() {
    local method="$1" path="$2" body="${3:-}"
    local args=(-s -X "$method" -H "X-SLURM-USER-TOKEN: $TOKEN" -H "Content-Type: application/json")
    [[ -n "$body" ]] && args+=(-d "$body")
    local resp
    resp=$(curl "${args[@]}" "$URL/slurmdb/$API/$path")
    local errors
    errors=$(echo "$resp" | jq -r '.errors // [] | map(.description) | join(", ")' 2>/dev/null || true)
    if [[ -n "$errors" ]]; then
        echo "  ERROR: $errors" >&2
        return 1
    fi
    echo "$resp"
}

step() { echo; echo "=== $* ==="; }
ok()   { echo "  ok: $*"; }

# ---------------------------------------------------------------------------
# QOS
# Covers: description-only, priority, max_wall_pj, preempt, full combination
# ---------------------------------------------------------------------------
step "QOS"

api POST qos/ '{"qos":[{"name":"tacc-qos-minimal","description":"Minimal test QOS"}]}' > /dev/null
ok "tacc-qos-minimal (description only)"

api POST qos/ '{"qos":[{"name":"tacc-qos-priority","description":"Priority test QOS","priority":{"number":200,"set":true}}]}' > /dev/null
ok "tacc-qos-priority (priority=200)"

api POST qos/ '{
  "qos":[{
    "name":"tacc-qos-walltime",
    "description":"Walltime test QOS",
    "limits":{"max":{"wall_clock":{"per":{"job":{"number":480,"set":true}}}}}
  }]
}' > /dev/null
ok "tacc-qos-walltime (max_wall_pj=480)"

api POST qos/ '{
  "qos":[{
    "name":"tacc-qos-preempt",
    "description":"Preempt test QOS",
    "priority":{"number":300,"set":true},
    "preempt":{"list":["tacc-qos-minimal"],"mode":["CANCEL"]}
  }]
}' > /dev/null
ok "tacc-qos-preempt (priority=300, preempts tacc-qos-minimal)"

api POST qos/ '{
  "qos":[{
    "name":"tacc-qos-full",
    "description":"Full attribute test QOS",
    "priority":{"number":150,"set":true},
    "limits":{"max":{"wall_clock":{"per":{"job":{"number":1440,"set":true}}}}},
    "preempt":{"list":["tacc-qos-minimal","tacc-qos-priority"],"mode":["CANCEL"]}
  }]
}' > /dev/null
ok "tacc-qos-full (priority + walltime + preempt)"

# ---------------------------------------------------------------------------
# Accounts
# Covers: description, organization, parent_account, fairshare, default_qos,
#         allowed_qos, max_jobs, and combinations thereof
# ---------------------------------------------------------------------------
step "Accounts"

api POST accounts_association/ "{
  \"association_condition\":{\"accounts\":[\"tacc-acct-minimal\"],\"clusters\":[\"$CLUSTER\"]},
  \"account\":{\"description\":\"Minimal test account\"}
}" > /dev/null
ok "tacc-acct-minimal (description only)"

api POST accounts_association/ "{
  \"association_condition\":{\"accounts\":[\"tacc-acct-org\"],\"clusters\":[\"$CLUSTER\"]},
  \"account\":{\"description\":\"Org test account\",\"organization\":\"testorg\"}
}" > /dev/null
ok "tacc-acct-org (description + organization)"

api POST accounts_association/ "{
  \"association_condition\":{
    \"accounts\":[\"tacc-acct-fairshare\"],\"clusters\":[\"$CLUSTER\"],
    \"association\":{\"fairshare\":500}
  },
  \"account\":{\"description\":\"Fairshare test account\"}
}" > /dev/null
ok "tacc-acct-fairshare (fairshare=500)"

api POST accounts_association/ "{
  \"association_condition\":{
    \"accounts\":[\"tacc-acct-qos\"],\"clusters\":[\"$CLUSTER\"],
    \"association\":{\"defaultqos\":\"tacc-qos-minimal\",\"qoslevel\":[\"tacc-qos-minimal\",\"tacc-qos-priority\"]}
  },
  \"account\":{\"description\":\"QOS test account\"}
}" > /dev/null
ok "tacc-acct-qos (default_qos + allowed_qos)"

# Child account — parent must exist first (tacc-acct-org created above)
api POST accounts_association/ "{
  \"association_condition\":{\"accounts\":[\"tacc-acct-child\"],\"clusters\":[\"$CLUSTER\"]},
  \"account\":{\"description\":\"Child test account\",\"parent\":\"tacc-acct-org\"}
}" > /dev/null
ok "tacc-acct-child (parent=tacc-acct-org)"

api POST accounts_association/ "{
  \"association_condition\":{
    \"accounts\":[\"tacc-acct-full\"],\"clusters\":[\"$CLUSTER\"],
    \"association\":{\"fairshare\":200,\"defaultqos\":\"tacc-qos-minimal\",\"qoslevel\":[\"tacc-qos-minimal\",\"tacc-qos-priority\"]}
  },
  \"account\":{\"description\":\"Full attribute test account\",\"organization\":\"testorg\"}
}" > /dev/null
ok "tacc-acct-full (all attributes)"

# ---------------------------------------------------------------------------
# Users
# Covers: single association, fairshare, qos limits, multiple associations
# ---------------------------------------------------------------------------
step "Users"

# Minimal: one association, no limits
api POST users_association/ "{
  \"association_condition\":{\"users\":[\"tacc-user-minimal\"],\"accounts\":[\"tacc-acct-minimal\"]},
  \"user\":{}
}" > /dev/null
ok "tacc-user-minimal (single association)"

# With fairshare on association
api POST users_association/ "{
  \"association_condition\":{\"users\":[\"tacc-user-fairshare\"],\"accounts\":[\"tacc-acct-fairshare\"]},
  \"user\":{}
}" > /dev/null
api POST associations/ "{
  \"associations\":[{\"user\":\"tacc-user-fairshare\",\"account\":\"tacc-acct-fairshare\",\"cluster\":\"$CLUSTER\",\"shares_raw\":100}]
}" > /dev/null
ok "tacc-user-fairshare (fairshare=100 on association)"

# With QOS limits on association
api POST users_association/ "{
  \"association_condition\":{\"users\":[\"tacc-user-qos\"],\"accounts\":[\"tacc-acct-qos\"]},
  \"user\":{}
}" > /dev/null
api POST associations/ "{
  \"associations\":[{
    \"user\":\"tacc-user-qos\",\"account\":\"tacc-acct-qos\",\"cluster\":\"$CLUSTER\",
    \"default\":{\"qos\":\"tacc-qos-minimal\"},
    \"qos\":[\"tacc-qos-minimal\",\"tacc-qos-priority\"]
  }]
}" > /dev/null
ok "tacc-user-qos (default_qos + qos list)"

# Multiple associations
api POST users_association/ "{
  \"association_condition\":{\"users\":[\"tacc-user-multi\"],\"accounts\":[\"tacc-acct-minimal\"]},
  \"user\":{}
}" > /dev/null
api POST users_association/ "{
  \"association_condition\":{\"users\":[\"tacc-user-multi\"],\"accounts\":[\"tacc-acct-org\"]},
  \"user\":{}
}" > /dev/null
ok "tacc-user-multi (two associations)"

echo
echo "Done. Populate complete."
echo "Run ./test/run_import_tests.sh to import and verify."
