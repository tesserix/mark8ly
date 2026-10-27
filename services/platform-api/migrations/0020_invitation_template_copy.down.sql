UPDATE email_templates
SET html_body = replace(replace(html_body,
        '{{if .Inviter}}{{.Inviter}} invited you{{else}}You&rsquo;ve been invited{{end}} to join {{.TenantName}} on Mark8ly as {{.Role}}.',
        '{{.Inviter}} invited you to join {{.TenantName}} on Mark8ly as a {{.Role}}.'),
        'on Mark8ly as <strong', 'on Mark8ly as a <strong'),
    text_body = replace(text_body, 'on Mark8ly as {{.Role}}.', 'on Mark8ly as a {{.Role}}.'),
    updated_at = now()
WHERE key = 'invitation' AND updated_by = 'embedded-seed';
