# Multi-stage Dockerfile for the storefront Next.js app.
# Builds on tesserix company-owned base images — libc6-compat,
# non-root uid 10001 (user "nextjs"), tini PID 1, corepack, and
# weekly upstream CVE patches come from the base.
#
# Build context MUST be the monorepo root. Identical pattern to
# apps/admin and apps/onboarding — only workspace name + port differ.

# ─── deps ──────────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-node-builder-24@sha256:c5b173e90d748e60496f7b7be877e767226867c974cf322bec634f04beb45819 AS deps
USER 0:0
WORKDIR /src

COPY package.json package-lock.json ./
COPY apps/onboarding/package.json ./apps/onboarding/
COPY apps/admin/package.json ./apps/admin/
COPY apps/storefront/package.json ./apps/storefront/
COPY services/platform-api/package.json ./services/platform-api/
COPY services/auth-bff/package.json ./services/auth-bff/
COPY packages/eslint-config/package.json ./packages/eslint-config/
COPY packages/typescript-config/package.json ./packages/typescript-config/
COPY packages/ui/package.json ./packages/ui/

# @tesserix/* now comes from the public npm registry, so this needs no
# credential. The scoped-registry .npmrc and its PACKAGE_READ_TOKEN
# BuildKit secret are gone with it — an unauthenticated pull is one less
# thing that can expire.
RUN npm ci --no-audit --no-fund --legacy-peer-deps

# ─── builder ───────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-node-builder-24@sha256:c5b173e90d748e60496f7b7be877e767226867c974cf322bec634f04beb45819 AS builder
USER 0:0
WORKDIR /src
COPY --from=deps /src/node_modules ./node_modules
# npm does not always hoist a workspace dependency to the root. When two
# workspaces resolve a package the root tree cannot satisfy, it lands in
# apps/<app>/node_modules instead — and copying only /src/node_modules
# silently drops it, which surfaces much later as "Module not found" during
# next build rather than as an install error. Carry the per-workspace trees
# too; the context excludes node_modules, so the COPY below cannot clobber
# them.
COPY --from=deps /src/apps ./apps
COPY . .

# Public Next.js env vars inlined at build time. Storefront currently
# has no public config, but the ARGs are kept for parity so all three
# Next apps accept the same build interface.
ARG REUSABLE_PUBLIC_BUILD_ARG_1=""
ARG REUSABLE_PUBLIC_BUILD_ARG_2=""
ARG REUSABLE_PUBLIC_BUILD_ARG_3=""
ARG REUSABLE_PUBLIC_BUILD_ARG_4=""
ARG REUSABLE_PUBLIC_BUILD_ARG_5=""
ARG REUSABLE_PUBLIC_BUILD_ARG_6=""
ARG REUSABLE_PUBLIC_BUILD_ARG_7=""
ARG REUSABLE_PUBLIC_BUILD_ARG_8=""
# Selects the auth backend. NEXT_PUBLIC_* is inlined into the bundle at
# BUILD time (server actions included), so this cannot be flipped by a
# chart env var — it must be passed here. See .github/ci/container-images.json.
ARG NEXT_PUBLIC_AUTH_PROVIDER="zitadel"
ENV NEXT_PUBLIC_GIP_PROJECT_ID=$REUSABLE_PUBLIC_BUILD_ARG_1 \
    NEXT_PUBLIC_GIP_TENANT_ID=$REUSABLE_PUBLIC_BUILD_ARG_2 \
    NEXT_PUBLIC_GIP_API_KEY=$REUSABLE_PUBLIC_BUILD_ARG_3 \
    NEXT_PUBLIC_GOOGLE_CLIENT_ID=$REUSABLE_PUBLIC_BUILD_ARG_5 \
    NEXT_PUBLIC_MARKETING_URL=$REUSABLE_PUBLIC_BUILD_ARG_6 \
    NEXT_PUBLIC_ADMIN_URL_TEMPLATE=$REUSABLE_PUBLIC_BUILD_ARG_7 \
    NEXT_PUBLIC_AUTH_PROVIDER=$NEXT_PUBLIC_AUTH_PROVIDER \
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
    npm run build --workspace=@mark8ly/storefront

# ─── runtime ───────────────────────────────────────────────────────────
# base-node-runtime-24: libc6-compat, tini PID 1, USER 10001 (nextjs).
FROM ghcr.io/tesserix/base-node-runtime-24@sha256:6e503bc4b951eeb065b9ff8c4a0d53bfd240b9e974709c79b56f163b9d70b683 AS runtime
WORKDIR /app

ENV PORT=4203

COPY --from=builder --chown=10001:10001 /src/apps/storefront/.next/standalone ./
COPY --from=builder --chown=10001:10001 /src/apps/storefront/.next/static ./apps/storefront/.next/static
COPY --from=builder --chown=10001:10001 /src/apps/storefront/public ./apps/storefront/public

EXPOSE 4203
# base-node-runtime-24's ENTRYPOINT is already `tini --`.
CMD ["node", "apps/storefront/server.js"]
