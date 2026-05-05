\set ON_ERROR_STOP on
UPDATE store_branding SET custom_css = $css$
/* Hero text legibility — flips heading, subheading, eyebrow, and CTAs to white */
img.opacity-70 ~ div h1.text-foreground {
  color: #ffffff !important;
  text-shadow: 0 2px 14px rgba(0,0,0,0.55), 0 0 2px rgba(0,0,0,0.4);
}
img.opacity-70 ~ div p.text-foreground-secondary {
  color: #f5f2eb !important;
  text-shadow: 0 1px 10px rgba(0,0,0,0.45);
}
img.opacity-70 ~ div p.text-accent {
  color: #ffffff !important;
  text-shadow: 0 1px 8px rgba(0,0,0,0.5);
  opacity: 0.95;
}
img.opacity-70 ~ div a.text-foreground,
img.opacity-70 ~ div a.text-foreground-secondary {
  color: #ffffff !important;
  border-bottom-color: rgba(255,255,255,0.9) !important;
  text-shadow: 0 1px 8px rgba(0,0,0,0.45);
}
$css$, updated_at = NOW()
WHERE tenant_id = '8c302556-b647-4824-8ce4-73f547ca456e';
