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

COPY package.json package-lock.json ./
COPY apps/onboarding/package.json ./apps/onboarding/
COPY apps/admin/package.json ./apps/admin/
COPY apps/storefront/package.json ./apps/storefront/
COPY services/platform-api/package.json ./services/platform-api/
COPY services/auth-bff/package.json ./services/auth-bff/
COPY packages/eslint-config/package.json ./packages/eslint-config/
COPY packages/typescript-config/package.json ./packages/typescript-config/
COPY packages/ui/package.json ./packages/ui/

RUN npm ci --no-audit --no-fund --legacy-peer-deps

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

RUN npm run build --workspace=@mark8ly/admin

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
