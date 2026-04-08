# Multi-stage Dockerfile for the storefront Next.js app.
#
# Build context MUST be the monorepo root. Identical pattern to
# apps/onboarding/Dockerfile and apps/admin/Dockerfile.

# ─── deps ──────────────────────────────────────────────────────────────
FROM node:22-alpine AS deps
WORKDIR /src
RUN apk add --no-cache libc6-compat

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

# ─── builder ───────────────────────────────────────────────────────────
FROM node:22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache libc6-compat
COPY --from=deps /src/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build --workspace=@mark8ly/storefront

# ─── runtime ───────────────────────────────────────────────────────────
FROM node:22-alpine AS runtime
WORKDIR /app
RUN apk add --no-cache libc6-compat \
    && addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=4203
ENV HOSTNAME=0.0.0.0

COPY --from=builder --chown=nextjs:nodejs /src/apps/storefront/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /src/apps/storefront/.next/static ./apps/storefront/.next/static
COPY --from=builder --chown=nextjs:nodejs /src/apps/storefront/public ./apps/storefront/public

USER nextjs
EXPOSE 4203

CMD ["node", "apps/storefront/server.js"]
