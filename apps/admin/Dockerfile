# Multi-stage Dockerfile for the admin Next.js app.
# Builds on tesserix company-owned base images — libc6-compat,
# non-root uid 10001 (user "nextjs"), tini PID 1, corepack, and
# weekly upstream CVE patches come from the base.
#
# Build context MUST be the monorepo root. apps/onboarding and
# apps/storefront follow the same pattern — only workspace name + port
# differ.

# ─── deps ──────────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-node-builder-22@sha256:6259770f19c5acc512f638abd60444013df2cb731c82d5a29903bb18b49362ea AS deps
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
RUN --mount=type=secret,id=PACKAGE_READ_TOKEN \
    NODE_AUTH_TOKEN="$(cat /run/secrets/PACKAGE_READ_TOKEN)" \
    npm ci --no-audit --no-fund --legacy-peer-deps

# ─── builder ───────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-node-builder-22@sha256:6259770f19c5acc512f638abd60444013df2cb731c82d5a29903bb18b49362ea AS builder
USER 0:0
WORKDIR /src
COPY --from=deps /src/node_modules ./node_modules
COPY . .

# Public Next.js env vars inlined at build time.
ARG REUSABLE_PUBLIC_BUILD_ARG_1=""
ARG REUSABLE_PUBLIC_BUILD_ARG_2=""
ARG REUSABLE_PUBLIC_BUILD_ARG_3=""
ARG REUSABLE_PUBLIC_BUILD_ARG_4=""
ARG REUSABLE_PUBLIC_BUILD_ARG_5=""
ARG REUSABLE_PUBLIC_BUILD_ARG_6=""
ARG REUSABLE_PUBLIC_BUILD_ARG_7=""
ARG REUSABLE_PUBLIC_BUILD_ARG_8=""
# Apple Services ID (web client). Empty => the Sign in with Apple button
# stays hidden (appleSignInEnabled in lib/config.ts). Set only once the
# Apple Developer Services ID exists AND its client secret is configured
# on the GIP apple.com provider — a button that 404s at Apple is worse
# than no button.
ARG NEXT_PUBLIC_APPLE_SERVICES_ID=""
ENV NEXT_PUBLIC_GIP_PROJECT_ID=$REUSABLE_PUBLIC_BUILD_ARG_1 \
    NEXT_PUBLIC_GIP_TENANT_ID=$REUSABLE_PUBLIC_BUILD_ARG_2 \
    NEXT_PUBLIC_GIP_API_KEY=$REUSABLE_PUBLIC_BUILD_ARG_3 \
    NEXT_PUBLIC_GOOGLE_CLIENT_ID=$REUSABLE_PUBLIC_BUILD_ARG_4 \
    NEXT_PUBLIC_MARKETING_URL=$REUSABLE_PUBLIC_BUILD_ARG_6 \
    NEXT_PUBLIC_ADMIN_URL_TEMPLATE=$REUSABLE_PUBLIC_BUILD_ARG_7 \
    NEXT_PUBLIC_ADMIN_LOGIN_ORIGIN=$REUSABLE_PUBLIC_BUILD_ARG_8 \
    NEXT_PUBLIC_APPLE_SERVICES_ID=$NEXT_PUBLIC_APPLE_SERVICES_ID \
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
# Fingerprint of the encryption key, passed by CI. Its only job is to
# land in this layer's cache key: BuildKit excludes secret CONTENTS from
# the cache key, so without this a rotated key would hit the cached build
# and keep baking the old one. Never the key itself — just a hash of it.
ARG REUSABLE_BUILD_CACHE_FP=""
RUN --mount=type=secret,id=APPLICATION_BUILD_SECRET \
    : "$REUSABLE_BUILD_CACHE_FP"; \
    NEXT_SERVER_ACTIONS_ENCRYPTION_KEY="$(cat /run/secrets/APPLICATION_BUILD_SECRET 2>/dev/null || true)"; \
    if [ -z "$NEXT_SERVER_ACTIONS_ENCRYPTION_KEY" ]; then \
      echo "WARNING: NEXT_SERVER_ACTIONS_ENCRYPTION_KEY is empty — Next.js"; \
      echo "will use a RANDOM per-build key, so Server Action IDs churn on"; \
      echo "every deploy. CI fails earlier on this; a local build without"; \
      echo "--secret is expected to land here."; \
    fi; \
    export NEXT_SERVER_ACTIONS_ENCRYPTION_KEY; \
    npm run build --workspace=@mark8ly/admin

# ─── runtime ───────────────────────────────────────────────────────────
# base-node-runtime-22: libc6-compat, tini PID 1, USER 10001 (nextjs).
FROM ghcr.io/tesserix/base-node-runtime-22@sha256:9a36aed7bef521cc497431f7653e20143749b9882b03d79b7a2a58f20ed20ef8 AS runtime
WORKDIR /app

ENV PORT=4202

COPY --from=builder --chown=10001:10001 /src/apps/admin/.next/standalone ./
COPY --from=builder --chown=10001:10001 /src/apps/admin/.next/static ./apps/admin/.next/static
COPY --from=builder --chown=10001:10001 /src/apps/admin/public ./apps/admin/public

EXPOSE 4202
# base-node-runtime-22's ENTRYPOINT is already `tini --`.
CMD ["node", "apps/admin/server.js"]
