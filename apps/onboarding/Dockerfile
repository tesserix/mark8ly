# Multi-stage Dockerfile for the onboarding Next.js app.
#
# Uses Next.js standalone output (output: "standalone" in next.config.ts)
# so the runtime image only contains the minimal Node server + the
# .next bundle. No node_modules duplication at runtime.
#
# IMPORTANT: build context MUST be the monorepo root (not apps/onboarding)
# because npm-workspaces resolves the lockfile + workspace package.jsons
# from the repo root.

# ─── deps: install workspace dependencies ──────────────────────────────
FROM node:22-alpine AS deps
WORKDIR /src
RUN apk add --no-cache libc6-compat

# Copy lockfile + every workspace package.json. Required for npm ci to
# resolve the workspace graph correctly.
COPY package.json package-lock.json ./
COPY apps/onboarding/package.json ./apps/onboarding/
COPY apps/admin/package.json ./apps/admin/
COPY apps/storefront/package.json ./apps/storefront/
COPY services/platform-api/package.json ./services/platform-api/
COPY services/auth-bff/package.json ./services/auth-bff/
COPY packages/eslint-config/package.json ./packages/eslint-config/
COPY packages/typescript-config/package.json ./packages/typescript-config/
COPY packages/ui/package.json ./packages/ui/

RUN npm ci --no-audit --no-fund

# ─── builder: build the onboarding workspace ───────────────────────────
FROM node:22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache libc6-compat
COPY --from=deps /src/node_modules ./node_modules
COPY . .

# Public Next.js env vars — these are INLINED into the client bundle
# at build time. Setting them as runtime env vars in Kubernetes is a
# no-op for NEXT_PUBLIC_* — the bundle is already frozen. CI passes
# these via `docker buildx build --build-arg NEXT_PUBLIC_*=...` from
# GH Actions secrets so the baked bundle has the real values.
ARG NEXT_PUBLIC_GIP_PROJECT_ID=""
ARG NEXT_PUBLIC_GIP_TENANT_ID=""
ARG NEXT_PUBLIC_GIP_API_KEY=""
ARG NEXT_PUBLIC_GOOGLE_CLIENT_ID=""
ARG NEXT_PUBLIC_MARKETING_URL=""
ARG NEXT_PUBLIC_ADMIN_URL_TEMPLATE=""
ENV NEXT_PUBLIC_GIP_PROJECT_ID=$NEXT_PUBLIC_GIP_PROJECT_ID
ENV NEXT_PUBLIC_GIP_TENANT_ID=$NEXT_PUBLIC_GIP_TENANT_ID
ENV NEXT_PUBLIC_GIP_API_KEY=$NEXT_PUBLIC_GIP_API_KEY
ENV NEXT_PUBLIC_GOOGLE_CLIENT_ID=$NEXT_PUBLIC_GOOGLE_CLIENT_ID
ENV NEXT_PUBLIC_MARKETING_URL=$NEXT_PUBLIC_MARKETING_URL
ENV NEXT_PUBLIC_ADMIN_URL_TEMPLATE=$NEXT_PUBLIC_ADMIN_URL_TEMPLATE

ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build --workspace=@mark8ly/onboarding

# ─── runtime: minimal node + the standalone bundle ─────────────────────
FROM node:22-alpine AS runtime
WORKDIR /app
RUN apk add --no-cache libc6-compat \
    && addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=4201
ENV HOSTNAME=0.0.0.0

# Next.js standalone output for a workspace ends up at
# apps/onboarding/.next/standalone/. Inside that, the directory layout
# is preserved relative to the monorepo root, so the entrypoint is
# apps/onboarding/server.js.
COPY --from=builder --chown=nextjs:nodejs /src/apps/onboarding/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /src/apps/onboarding/.next/static ./apps/onboarding/.next/static
COPY --from=builder --chown=nextjs:nodejs /src/apps/onboarding/public ./apps/onboarding/public

USER nextjs
EXPOSE 4201

CMD ["node", "apps/onboarding/server.js"]
