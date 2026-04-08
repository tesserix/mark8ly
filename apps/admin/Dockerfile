# Multi-stage Dockerfile for the admin Next.js app.
#
# Build context MUST be the monorepo root. See apps/onboarding/Dockerfile
# for the design rationale — both Next.js apps follow the identical
# pattern, only the workspace name + ports differ.

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

# Public Next.js env vars inlined at build time. See apps/onboarding/Dockerfile
# for the rationale.
ARG NEXT_PUBLIC_GIP_PROJECT_ID=""
ARG NEXT_PUBLIC_GIP_TENANT_ID=""
ARG NEXT_PUBLIC_GIP_API_KEY=""
ARG NEXT_PUBLIC_GOOGLE_CLIENT_ID=""
ARG NEXT_PUBLIC_MARKETING_URL=""
ENV NEXT_PUBLIC_GIP_PROJECT_ID=$NEXT_PUBLIC_GIP_PROJECT_ID
ENV NEXT_PUBLIC_GIP_TENANT_ID=$NEXT_PUBLIC_GIP_TENANT_ID
ENV NEXT_PUBLIC_GIP_API_KEY=$NEXT_PUBLIC_GIP_API_KEY
ENV NEXT_PUBLIC_GOOGLE_CLIENT_ID=$NEXT_PUBLIC_GOOGLE_CLIENT_ID
ENV NEXT_PUBLIC_MARKETING_URL=$NEXT_PUBLIC_MARKETING_URL

ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build --workspace=@mark8ly/admin

# ─── runtime ───────────────────────────────────────────────────────────
FROM node:22-alpine AS runtime
WORKDIR /app
RUN apk add --no-cache libc6-compat \
    && addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 nextjs

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=4202
ENV HOSTNAME=0.0.0.0

COPY --from=builder --chown=nextjs:nodejs /src/apps/admin/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /src/apps/admin/.next/static ./apps/admin/.next/static
COPY --from=builder --chown=nextjs:nodejs /src/apps/admin/public ./apps/admin/public

USER nextjs
EXPOSE 4202

CMD ["node", "apps/admin/server.js"]
