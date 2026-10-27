-- Invitation email copy fixes (article before the role, blank inviter in the
-- preheader). email_templates rows are seeded once from the embedded files and
-- never re-seeded, so the embedded edit alone never reaches an existing DB.
-- Only rows nobody has edited in the console are touched.
UPDATE email_templates
SET html_body = replace(replace(html_body,
        '{{.Inviter}} invited you to join {{.TenantName}} on Mark8ly as a {{.Role}}.',
        '{{if .Inviter}}{{.Inviter}} invited you{{else}}You&rsquo;ve been invited{{end}} to join {{.TenantName}} on Mark8ly as {{.Role}}.'),
        'on Mark8ly as a <strong', 'on Mark8ly as <strong'),
    text_body = replace(text_body, 'on Mark8ly as a {{.Role}}.', 'on Mark8ly as {{.Role}}.'),
    updated_at = now()
WHERE key = 'invitation' AND updated_by = 'embedded-seed';
