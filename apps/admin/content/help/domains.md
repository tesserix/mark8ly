---
title: "Custom Domains"
category: "Store Setup"
order: 11
---

Every Mark8ly store comes with a free subdomain at `yourstore.mark8ly.com`, ready immediately. When you're ready to use your own domain, **Settings → Domains** walks you through it. This article covers both setup methods, the DNS records you'll need, and the SSL flow.

## Default domain

Your `yourstore.mark8ly.com` URL is:

- Active immediately at store creation
- HTTPS by default
- Backed by Mark8ly's CDN for fast loading worldwide

It's a perfectly fine starting point while you build out the rest of your store.

## Connecting a custom domain

To use your own domain (e.g. `www.yourshop.com`), open **Settings → Domains**, enter your domain, and pick one of two setup methods:

| Method | What it needs | Best for |
| --- | --- | --- |
| **Manual (CNAME)** | DNS records you add at your registrar | Any DNS provider — full control |
| **Cloudflare (auto)** | A scoped Cloudflare API token | Cloudflare zones — we add the record for you |

> Once your custom domain is verified and active, the platform automatically retires the `yourstore.mark8ly.com` URL. Any traffic that lands there is permanently redirected (HTTP 301) to your custom domain, so old bookmarks, search-engine entries, and shared links keep working. You stay signed in across the redirect.

## Manual setup — DNS records

After you click **Add domain** with the **Manual** method selected, the page shows the exact records you need. There are two required records and one optional record.

### 1. Route traffic to your storefront

Pick **one** of the following:

- **Option A — A record** (works for apex domains like `yourshop.com`): Type `A`, Name `yourshop.com`, Value = the IP shown on the page.
- **Option B — CNAME** (cleaner if your DNS provider allows CNAME on this name): Type `CNAME`, Name `yourshop.com`, Value = the target shown on the page.

### 2. Delegate SSL certificate issuance

Add a `CNAME` record:

- **Name:** `_acme-challenge.yourshop.com`
- **Target:** `_acme-challenge.yourshop.com.acme.mark8ly.com`

This lets Mark8ly issue and renew a free Let's Encrypt certificate without you ever sharing DNS credentials.

### 3. (Optional) Admin subdomain

If you'd like a branded admin URL like `admin.yourshop.com`, add an `A` record at `admin.yourshop.com` pointing to the same IP from step 1. Otherwise, skip this and keep using the default admin URL.

> DNS changes can take up to **48 hours** to propagate, though most registrars publish within an hour. You can check propagation at [dnschecker.org](https://dnschecker.org). Once the required records are live, click **Verify** on the domain card.

## Automated setup with Cloudflare

The **Cloudflare (auto)** method asks for a Cloudflare API token. Mark8ly uses the token only to add the routing CNAME on your zone — never to read DNS for any other zone, and never to make changes outside your domain.

The token is stored in **Google Cloud Secret Manager** scoped to your tenant and is never written to a database or to logs.

### Token permissions

The token needs **two** permissions:

- `Zone → DNS → Edit` — lets us add the CNAME record automatically.
- `Zone → Zone → Read` — lets us find your zone by domain name.

Under **Zone Resources**, scope the token to *only the zone for this domain*. Do not grant Account-level permissions, and do not include other zones.

### Creating the token

1. Sign in to [dash.cloudflare.com → My Profile → API Tokens](https://dash.cloudflare.com/profile/api-tokens).
2. Click **Create Token** and pick the **Edit zone DNS** template.
3. Under *Zone Resources*, choose **Include → Specific zone → your domain**.
4. (Recommended) Set an expiry date and add an IP allowlist for safety.
5. Click **Continue to summary**, then **Create Token**, and copy the token value. **Cloudflare only shows it once.**
6. Paste the token into the **Cloudflare API token** field on the Domains page and click **Add domain**.

### What happens after you submit

1. We securely store the token in Google Cloud Secret Manager under your tenant.
2. We look up your zone on Cloudflare and add a DNS-only CNAME pointing your domain to our edge.
3. The domain enters the **verifying** state while DNS propagates.
4. Once verified, we automatically issue a Let's Encrypt SSL certificate.

> If something goes wrong, the domain card shows an inline error with a hint on what to check. The most common cause is a token without the right permissions or scoped to the wrong zone — re-create it and click **Verify** again.

### Revoking access

You can revoke the token at any time from your Cloudflare dashboard. If you remove the domain from Mark8ly, we delete the secret and the DNS record we created.

## SSL certificates

Mark8ly automatically provisions and renews SSL certificates for your custom domain via Let's Encrypt. Once DNS is properly configured, the certificate is issued within minutes and renewed continuously.

If the certificate fails to provision, double-check your DNS records — the most common issue is a missing or incorrect `_acme-challenge` CNAME. The domain settings page shows the current status and any errors to help you troubleshoot.

## Multiple domains

You can connect more than one custom domain to a single store — useful if you own variations like `yourshop.com` and `yourshop.co.uk`.

- One domain is set as **primary** (the canonical URL used in emails and social shares).
- Additional domains automatically redirect to the primary.

## Best practices

- Use a `www` subdomain as your primary domain rather than the bare apex. This gives you more flexibility with DNS and CDN routing. Most registrars support forwarding the bare domain to the `www` version.
- Keep your domain registration current and **enable auto-renewal**. A lapsed domain can be registered by someone else, and you'd lose your store URL plus any SEO authority you've built.
