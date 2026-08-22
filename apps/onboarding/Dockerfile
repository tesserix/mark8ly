# Multi-stage Dockerfile for the onboarding Next.js app.
# Builds on tesserix company-owned base images — libc6-compat,
# non-root uid 10001 (user "nextjs"), tini PID 1, corepack, and
# weekly upstream CVE patches come from the base.
#
# Uses Next.js standalone output so the runtime image only contains
# the minimal Node server + the .next bundle.
#
# Build context MUST be the monorepo root (not apps/onboarding) —
# npm workspaces resolves the lockfile + workspace package.jsons from
# the repo root.

# ─── deps ──────────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-node-builder-22@sha256:ac0254526efbc5bd4079193a010e9087ea722eada2f4bede2db1817b900b7a01 AS deps
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

# Root npm ci resolves the entire workspace tree, which now includes
# @tesserix/otto-widget under admin/storefront. The token is mounted as
# a BuildKit secret so it never lands in a layer.
RUN --mount=type=secret,id=PACKAGE_READ_TOKEN \
    NODE_AUTH_TOKEN="$(cat /run/secrets/PACKAGE_READ_TOKEN)" \
    npm ci --no-audit --no-fund --legacy-peer-deps

# ─── builder ───────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-node-builder-22@sha256:ac0254526efbc5bd4079193a010e9087ea722eada2f4bede2db1817b900b7a01 AS builder
USER 0:0
WORKDIR /src
COPY --from=deps /src/node_modules ./node_modules
COPY . .

# Public Next.js env vars — INLINED into the client bundle at build
# time. Setting them as runtime env vars in Kubernetes is a no-op for
# NEXT_PUBLIC_* — the bundle is already frozen. CI passes these via
# `docker buildx build --build-arg NEXT_PUBLIC_*=...` from GH Actions
# secrets so the baked bundle has the real values.
ARG REUSABLE_PUBLIC_BUILD_ARG_1=""
ARG REUSABLE_PUBLIC_BUILD_ARG_2=""
ARG REUSABLE_PUBLIC_BUILD_ARG_3=""
ARG REUSABLE_PUBLIC_BUILD_ARG_4=""
ARG REUSABLE_PUBLIC_BUILD_ARG_5=""
ARG REUSABLE_PUBLIC_BUILD_ARG_6=""
ARG REUSABLE_PUBLIC_BUILD_ARG_7=""
ARG REUSABLE_PUBLIC_BUILD_ARG_8=""
ARG NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID=""
ENV NEXT_PUBLIC_GIP_PROJECT_ID=$REUSABLE_PUBLIC_BUILD_ARG_1 \
    NEXT_PUBLIC_GIP_TENANT_ID=$REUSABLE_PUBLIC_BUILD_ARG_2 \
    NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID=$NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID \
    NEXT_PUBLIC_GIP_API_KEY=$REUSABLE_PUBLIC_BUILD_ARG_3 \
    NEXT_PUBLIC_GOOGLE_CLIENT_ID=$REUSABLE_PUBLIC_BUILD_ARG_4 \
    NEXT_PUBLIC_MARKETING_URL=$REUSABLE_PUBLIC_BUILD_ARG_6 \
    NEXT_PUBLIC_ADMIN_URL_TEMPLATE=$REUSABLE_PUBLIC_BUILD_ARG_7 \
    NEXT_TELEMETRY_DISABLED=1

# Pins the Server Action ID hash salt so action IDs stay stable across
# builds. See apps/admin/Dockerfile for the full rationale and the
# rotation/cache caveat.
# Cache-key carrier for the encryption key — see apps/admin/Dockerfile.
ARG REUSABLE_BUILD_CACHE_FP=""
RUN --mount=type=secret,id=APPLICATION_BUILD_SECRET \
    : "$REUSABLE_BUILD_CACHE_FP"; \
    NEXT_SERVER_ACTIONS_ENCRYPTION_KEY="$(cat /run/secrets/APPLICATION_BUILD_SECRET 2>/dev/null || true)"; \
    if [ -z "$NEXT_SERVER_ACTIONS_ENCRYPTION_KEY" ]; then \
      echo "WARNING: NEXT_SERVER_ACTIONS_ENCRYPTION_KEY is empty — Next.js"; \
      echo "will use a RANDOM per-build key, so Server Action IDs churn on"; \
      echo "every deploy. In CI this means the repo secret is missing."; \
    fi; \
    export NEXT_SERVER_ACTIONS_ENCRYPTION_KEY; \
    npm run build --workspace=@mark8ly/onboarding

# ─── runtime ───────────────────────────────────────────────────────────
# base-node-runtime-22: libc6-compat, tini PID 1, USER 10001 (nextjs).
# Next.js standalone output for a workspace ends up at
# apps/onboarding/.next/standalone/ with the monorepo directory layout
# preserved, so the entrypoint is apps/onboarding/server.js.
FROM ghcr.io/tesserix/base-node-runtime-22@sha256:41ad466bd1f42a02eca0a3d8c7022d2a42438b9bba31c59f65001af283feb330 AS runtime
WORKDIR /app

ENV PORT=4201

COPY --from=builder --chown=10001:10001 /src/apps/onboarding/.next/standalone ./
COPY --from=builder --chown=10001:10001 /src/apps/onboarding/.next/static ./apps/onboarding/.next/static
COPY --from=builder --chown=10001:10001 /src/apps/onboarding/public ./apps/onboarding/public

EXPOSE 4201
# base-node-runtime-22's ENTRYPOINT is already `tini --`.
CMD ["node", "apps/onboarding/server.js"]
