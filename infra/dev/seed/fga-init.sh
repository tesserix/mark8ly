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

# Convert the .fga DSL to JSON. Phase A keeps it simple by hand-encoding the
# minimal model directly. (Phase D may swap to `fga model write` from the CLI.)
MODEL_JSON='{
  "schema_version": "1.1",
  "type_definitions": [
    { "type": "user" },
    {
      "type": "tenant",
      "relations": {
        "member": { "this": {} },
        "owner": { "this": {} }
      },
      "metadata": {
        "relations": {
          "member": { "directly_related_user_types": [{ "type": "user" }] },
          "owner":  { "directly_related_user_types": [{ "type": "user" }] }
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
