/**
 * Wire types for the tenant SSO configuration (#820).
 *
 * Source of truth:
 *   services/marketplace-api/internal/handlers/admin/sso_config.go
 *   services/marketplace-api/internal/sso/repository.go  (Validate)
 */
import { z } from 'zod'

/**
 * The three metadata fields an OIDC login actually needs.
 *
 * Not a guess: `sso.Validate` requires exactly these, and
 * `BuildOIDCRelyingParty` reads issuer and client_id while the relying-party
 * cache reads client_secret_ref. `discovery_url` used to be required and is
 * read by nothing — discovery comes from the issuer — so it is not collected
 * here (#820).
 */
export const ssoMetadataSchema = z.object({
  issuer: z.string().default(''),
  client_id: z.string().default(''),
  /**
   * A PATH into the secret store, never the secret itself. The server
   * redacts it on read, so what comes back is "[redacted]" once saved —
   * see redactConfig.
   */
  client_secret_ref: z.string().default(''),
  redirect_uri: z.string().default(''),
  scopes: z.string().default(''),
})

export const ssoConfigSchema = z.object({
  tenant_id: z.string(),
  provider: z.string(),
  metadata: ssoMetadataSchema,
  attr_mapping: z.record(z.string(), z.string()).default({}),
  enabled: z.boolean(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
})

export type SSOConfig = z.infer<typeof ssoConfigSchema>

/**
 * The server's answer to "Test connection".
 *
 * `checked` says HOW far the test got: "idp" means the identity provider was
 * reached and the client secret read; "configuration" means only the shape was
 * validated, because that build has no way to reach an IdP. Reporting a pass
 * without saying which would let a shape check read as a working login.
 */
export const ssoTestResultSchema = z.object({
  ok: z.boolean(),
  provider: z.string().default(''),
  checked: z.enum(['idp', 'configuration']).optional(),
  error: z.string().optional(),
})

export type SSOTestResult = z.infer<typeof ssoTestResultSchema>

/** The value the form submits. */
export interface SSOConfigInput {
  provider: 'oidc'
  metadata: {
    issuer: string
    client_id: string
    client_secret_ref: string
    redirect_uri?: string
    scopes?: string
  }
  enabled: boolean
}

/**
 * The placeholder the server returns in place of a saved secret reference.
 * Submitting it back would store the literal string, so the form treats it as
 * "unchanged" and omits the field.
 */
export const REDACTED = '[redacted]'
