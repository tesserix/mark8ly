/**
 * Single sign-on settings copy (#820).
 *
 * Voice per the design context: calm, editorial, precise. This screen asks a
 * merchant for values they must copy out of another system, so every field
 * says where the value comes from rather than restating its own name.
 *
 * The one rule the sentences follow: never claim more than the server checked.
 * "Test connection" reports whether the identity provider was actually
 * reached, and the copy distinguishes that from a shape check — because a
 * merchant who reads "looks good" and cannot sign in has been told the wrong
 * thing at the only moment they were listening.
 */
export const ssoCopy = {
  eyebrow: 'Settings',
  title: 'Single sign-on',
  description:
    'Let your team sign in to this dashboard with your own identity provider.',

  sectionTitle: 'OpenID Connect',
  sectionDescription:
    'Mark8ly supports OpenID Connect. Create an application in your identity provider, then paste its details here.',

  fields: {
    issuerLabel: 'Issuer URL',
    issuerHint:
      'The base URL your provider publishes its configuration under — Mark8ly reads the rest from there.',
    clientIdLabel: 'Client ID',
    clientIdHint: 'The application identifier your provider issued.',
    secretRefLabel: 'Client secret reference',
    secretRefHint:
      'The path where your client secret is stored. The secret itself never reaches Mark8ly’s database — contact us to have it stored.',
    secretRefKept: 'A secret reference is already saved. Leave this blank to keep it.',
    redirectLabel: 'Redirect URI (optional)',
    redirectHint: 'Add this to your provider’s allowed redirect list.',
    scopesLabel: 'Scopes (optional)',
    scopesHint: 'Space-separated. Defaults to openid email profile.',
    enabledLabel: 'Allow your team to sign in with this provider',
    enabledHint:
      'Turn this on once the connection test passes. Until then, everyone signs in as usual.',
  },

  save: 'Save configuration',
  saving: 'Saving…',
  saved: 'Configuration saved.',
  test: 'Test connection',
  testing: 'Testing…',
  remove: 'Remove configuration',
  removing: 'Removing…',
  removed: 'Configuration removed.',

  /** Shown when the test genuinely reached the identity provider. */
  testReachedIdP: 'Connected. Your identity provider answered and the client secret was read.',
  /**
   * Shown when only the shape was checked, which is not the same claim.
   * Saying "connected" here would be a promise this build cannot make.
   */
  testConfigOnly:
    'The configuration looks complete. This environment cannot reach your provider to confirm it.',
  testFailed: (reason: string) => `Not connected. ${reason}`,
  testError:
    'We couldn’t run the test just now. Your configuration has not been changed.',

  removeConfirmTitle: 'Remove single sign-on?',
  removeConfirmBody:
    'Your team will go back to signing in with their Mark8ly passwords. Anyone who has only ever signed in through your provider will need a password reset.',
  removeConfirmCta: 'Remove',
  cancel: 'Cancel',

  /**
   * The plan gate. The API answers 403 for a tenant whose plan lacks the
   * feature, and this says which plan it needs rather than "forbidden".
   */
  planGated:
    'Single sign-on is part of the Pro plan. Upgrade to connect your identity provider.',
  loadError: 'We couldn’t load your single sign-on settings.',
  saveError: 'We couldn’t save that. Check the values and try again.',

  /**
   * SAML. Stated once, plainly, rather than offered as an option that
   * answers 501 — the pricing page's claim was corrected alongside this
   * screen for the same reason.
   */
  samlNote:
    'SAML is not supported. If your provider only speaks SAML, contact us.',
} as const
