#!/bin/sh
# Creates the OpenFGA store and loads the authorization model.
# Idempotent: re-running just no-ops if the store already exists.
set -eu

apk add --no-cache curl jq >/dev/null 2>&1 || true

OPENFGA_URL="${OPENFGA_URL:-http://openfga:8080}"
STORE_NAME="mark8ly-platform"

# Wait until OpenFGA responds (depends_on healthcheck should already cover this).
until curl -fsS "${OPENFGA_URL}/healthz" >/dev/null 2>&1; do
  echo "fga-seed: waiting for openfga..."
  sleep 1
done

# Find or create the store.
STORE_ID=$(curl -fsS "${OPENFGA_URL}/stores" \
  | jq -r --arg name "$STORE_NAME" '.stores[] | select(.name == $name) | .id' \
  | head -n1)

if [ -z "$STORE_ID" ] || [ "$STORE_ID" = "null" ]; then
  echo "fga-seed: creating store '$STORE_NAME'"
  STORE_ID=$(curl -fsS -X POST "${OPENFGA_URL}/stores" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"${STORE_NAME}\"}" \
    | jq -r '.id')
fi
echo "fga-seed: store id = $STORE_ID"

# Phase O — Roles + RBAC.
#
# This JSON is the hand-encoded form of infra/openfga/model.fga. Keep
# the two in lockstep. Roles are hierarchical: owner ⊇ admin ⊇ staff ⊇
# viewer. Assigning a user as `owner` automatically satisfies Check()
# calls for admin/staff/viewer, matching the fake client's behaviour.
# The "member" relation is just an alias for viewer (the broadest role)
# and is kept as its own name for backwards compatibility with callers
# like auth-bff's CheckMembership.
MODEL_JSON='{
  "schema_version": "1.1",
  "type_definitions": [
    { "type": "user" },
    {
      "type": "tenant",
      "relations": {
        "owner": { "this": {} },
        "admin": {
          "union": {
            "child": [
              { "this": {} },
              { "computedUserset": { "relation": "owner" } }
            ]
          }
        },
        "staff": {
          "union": {
            "child": [
              { "this": {} },
              { "computedUserset": { "relation": "admin" } }
            ]
          }
        },
        "viewer": {
          "union": {
            "child": [
              { "this": {} },
              { "computedUserset": { "relation": "staff" } }
            ]
          }
        },
        "member": {
          "computedUserset": { "relation": "viewer" }
        },
        "can_view_settings": {
          "computedUserset": { "relation": "viewer" }
        },
        "can_edit_settings": {
          "computedUserset": { "relation": "admin" }
        },
        "can_invite_members": {
          "computedUserset": { "relation": "admin" }
        },
        "can_manage_stores": {
          "computedUserset": { "relation": "admin" }
        }
      },
      "metadata": {
        "relations": {
          "owner":  { "directly_related_user_types": [{ "type": "user" }] },
          "admin":  { "directly_related_user_types": [{ "type": "user" }] },
          "staff":  { "directly_related_user_types": [{ "type": "user" }] },
          "viewer": { "directly_related_user_types": [{ "type": "user" }] }
        }
      }
    },
    {
      "type": "store",
      "relations": {
        "parent":  { "this": {} },
        "owner":   { "this": {} },
        "manager": { "this": {} },
        "staff":   { "this": {} },
        "viewer":  { "this": {} },
        "tenant_admin": {
          "tupleToUserset": {
            "tupleset": { "relation": "parent" },
            "computedUserset": { "relation": "admin" }
          }
        },
        "tenant_owner": {
          "tupleToUserset": {
            "tupleset": { "relation": "parent" },
            "computedUserset": { "relation": "owner" }
          }
        },
        "member": {
          "union": {
            "child": [
              { "computedUserset": { "relation": "owner" } },
              { "computedUserset": { "relation": "manager" } },
              { "computedUserset": { "relation": "staff" } },
              { "computedUserset": { "relation": "viewer" } },
              {
                "tupleToUserset": {
                  "tupleset": { "relation": "parent" },
                  "computedUserset": { "relation": "member" }
                }
              }
            ]
          }
        },
        "can_view_store": {
          "union": {
            "child": [
              { "computedUserset": { "relation": "owner" } },
              { "computedUserset": { "relation": "manager" } },
              { "computedUserset": { "relation": "staff" } },
              { "computedUserset": { "relation": "viewer" } },
              {
                "tupleToUserset": {
                  "tupleset": { "relation": "parent" },
                  "computedUserset": { "relation": "member" }
                }
              }
            ]
          }
        },
        "can_edit_store_settings": {
          "union": {
            "child": [
              { "computedUserset": { "relation": "owner" } },
              { "computedUserset": { "relation": "manager" } },
              { "computedUserset": { "relation": "tenant_owner" } },
              { "computedUserset": { "relation": "tenant_admin" } }
            ]
          }
        },
        "can_manage_catalog": {
          "union": {
            "child": [
              { "computedUserset": { "relation": "owner" } },
              { "computedUserset": { "relation": "manager" } },
              { "computedUserset": { "relation": "staff" } },
              { "computedUserset": { "relation": "tenant_owner" } },
              { "computedUserset": { "relation": "tenant_admin" } }
            ]
          }
        }
      },
      "metadata": {
        "relations": {
          "parent":  { "directly_related_user_types": [{ "type": "tenant" }] },
          "owner":   { "directly_related_user_types": [{ "type": "user" }] },
          "manager": { "directly_related_user_types": [{ "type": "user" }] },
          "staff":   { "directly_related_user_types": [{ "type": "user" }] },
          "viewer":  { "directly_related_user_types": [{ "type": "user" }] }
        }
      }
    }
  ]
}'

echo "fga-seed: writing authorization model"
MODEL_ID=$(curl -fsS -X POST "${OPENFGA_URL}/stores/${STORE_ID}/authorization-models" \
  -H "Content-Type: application/json" \
  -d "$MODEL_JSON" \
  | jq -r '.authorization_model_id')
echo "fga-seed: model id = $MODEL_ID"

# Persist for downstream containers via a shared volume in a future iteration.
# For Phase A, the platform-api container reads FGA_STORE_ID from .env or its
# bind mount; we just print here.
echo "FGA_STORE_ID=$STORE_ID"
echo "FGA_MODEL_ID=$MODEL_ID"
