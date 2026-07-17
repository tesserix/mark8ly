# Multi-stage Dockerfile for the admin Next.js app.
# Builds on tesserix company-owned base images — libc6-compat,
# non-root uid 10001 (user "nextjs"), tini PID 1, corepack, and
# weekly upstream CVE patches come from the base.
#
# Build context MUST be the monorepo root. apps/onboarding and
# apps/storefront follow the same pattern — only workspace name + port
# differ.

# ─── deps ──────────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-node-builder-22:latest AS deps
USER 0:0
WORKDIR /src

COPY package.json package-lock.json .npmrc ./
COPY apps/onboarding/package.json ./apps/onboarding/
COPY apps/admin/package.json ./apps/admin/
COPY apps/storefront/package.json ./apps/storefront/
COPY services/platform-api/package.json ./services/platform-api/
COPY services/auth-bff/package.json ./services/auth-bff/
COPY packages/eslint-config/package.json ./packages/eslint-config/
COPY packages/typescript-config/package.json ./packages/typescript-config/
COPY packages/ui/package.json ./packages/ui/

# @tesserix/* deps live on GitHub Packages which always requires auth.
# The token is mounted as a BuildKit secret so it never lands in an
# image layer; npm reads it via the NODE_AUTH_TOKEN variable already
# interpolated into the root .npmrc.
RUN --mount=type=secret,id=NODE_AUTH_TOKEN \
    NODE_AUTH_TOKEN="$(cat /run/secrets/NODE_AUTH_TOKEN)" \
    npm ci --no-audit --no-fund --legacy-peer-deps

# ─── builder ───────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-node-builder-22:latest AS builder
USER 0:0
WORKDIR /src
COPY --from=deps /src/node_modules ./node_modules
COPY . .

# Public Next.js env vars inlined at build time.
ARG NEXT_PUBLIC_GIP_PROJECT_ID=""
ARG NEXT_PUBLIC_GIP_TENANT_ID=""
ARG NEXT_PUBLIC_GIP_API_KEY=""
ARG NEXT_PUBLIC_GOOGLE_CLIENT_ID=""
ARG NEXT_PUBLIC_MARKETING_URL=""
ARG NEXT_PUBLIC_ADMIN_URL_TEMPLATE=""
ARG NEXT_PUBLIC_ADMIN_LOGIN_ORIGIN=""
ENV NEXT_PUBLIC_GIP_PROJECT_ID=$NEXT_PUBLIC_GIP_PROJECT_ID \
    NEXT_PUBLIC_GIP_TENANT_ID=$NEXT_PUBLIC_GIP_TENANT_ID \
    NEXT_PUBLIC_GIP_API_KEY=$NEXT_PUBLIC_GIP_API_KEY \
    NEXT_PUBLIC_GOOGLE_CLIENT_ID=$NEXT_PUBLIC_GOOGLE_CLIENT_ID \
    NEXT_PUBLIC_MARKETING_URL=$NEXT_PUBLIC_MARKETING_URL \
    NEXT_PUBLIC_ADMIN_URL_TEMPLATE=$NEXT_PUBLIC_ADMIN_URL_TEMPLATE \
    NEXT_PUBLIC_ADMIN_LOGIN_ORIGIN=$NEXT_PUBLIC_ADMIN_LOGIN_ORIGIN \
    NEXT_TELEMETRY_DISABLED=1

# Server Action IDs are hashed with this key as the salt (Next.js wires
# it to serverReferenceHashSalt), so an unset key means a random one per
# build and every action ID churns on every deploy — breaking Server
# Actions in any tab opened before the rollout. Pinning it keeps IDs
# stable across builds whose action source didn't change.
#
# Mounted as a BuildKit secret rather than an ARG so it stays out of
# `docker history`. Next.js bakes the key into the build manifest, which
# is what the runtime falls back to — so no runtime env var is needed.
#
# Rotation caveat: BuildKit does not include secret contents in the layer
# cache key, and CI caches this stage (cache-from/to: type=gha). Changing
# only the secret will NOT rebuild this layer — bust the cache explicitly.
RUN --mount=type=secret,id=NEXT_SERVER_ACTIONS_ENCRYPTION_KEY \
    NEXT_SERVER_ACTIONS_ENCRYPTION_KEY="$(cat /run/secrets/NEXT_SERVER_ACTIONS_ENCRYPTION_KEY 2>/dev/null || true)"; \
    if [ -z "$NEXT_SERVER_ACTIONS_ENCRYPTION_KEY" ]; then \
      echo "############################################################"; \
      echo "WARNING: NEXT_SERVER_ACTIONS_ENCRYPTION_KEY is empty."; \
      echo "Next.js will generate a RANDOM key for this build, so every"; \
      echo "Server Action ID changes and any tab open across the next"; \
      echo "deploy will fail with UnrecognizedActionError."; \
      echo "In CI this means the repo secret is missing — fix it."; \
      echo "############################################################"; \
    fi; \
    export NEXT_SERVER_ACTIONS_ENCRYPTION_KEY; \
    npm run build --workspace=@mark8ly/admin

# ─── runtime ───────────────────────────────────────────────────────────
# base-node-runtime-22: libc6-compat, tini PID 1, USER 10001 (nextjs).
FROM ghcr.io/tesserix/base-node-runtime-22:latest AS runtime
WORKDIR /app

ENV PORT=4202

COPY --from=builder --chown=10001:10001 /src/apps/admin/.next/standalone ./
COPY --from=builder --chown=10001:10001 /src/apps/admin/.next/static ./apps/admin/.next/static
COPY --from=builder --chown=10001:10001 /src/apps/admin/public ./apps/admin/public

EXPOSE 4202
# base-node-runtime-22's ENTRYPOINT is already `tini --`.
CMD ["node", "apps/admin/server.js"]
