---
title: "Custom Domains"
category: "Store Setup"
order: 11
---

## Default Domain

Every Mark8ly store comes with a free subdomain at `yourstore.mark8ly.com`. This domain is active immediately when you create your store and requires no configuration. It is a good starting point while you set up the rest of your store.

The default subdomain uses HTTPS automatically and is backed by Mark8ly's content delivery network for fast loading anywhere in the world.

## Connecting a Custom Domain

To use your own domain like `www.yourshop.com`, navigate to **Settings > Domains** and enter your domain name. You can pick one of two setup methods:

- **Manual (CNAME)** — works with any DNS provider. We give you the records to copy in.
- **Cloudflare (auto)** — supply a scoped Cloudflare API token and we add the records for you.

Once your custom domain is verified and active, the platform automatically retires your `yourstore.mark8ly.com` URL: any traffic that lands there is permanently redirected (HTTP 301) to your custom domain so old bookmarks, search-engine entries, and shared links still work. You stay signed in across the redirect.

## Manual setup — what to add at your registrar

After you click **Add domain** with the Manual method selected, the page shows the exact records you need. There are two required records and one optional record.

**1. Route traffic to your storefront.** Pick one option:

- *Option A — A record* (works for apex domains like `yourshop.com`): point Type `A`, Name `yourshop.com` to the IP shown on the page.
- *Option B — CNAME* (cleaner if your DNS provider allows CNAME on this name): point Type `CNAME`, Name `yourshop.com` to the target shown on the page.

**2. Delegate SSL certificate issuance.** Add a `CNAME` record at `_acme-challenge.yourshop.com` pointing to `_acme-challenge.yourshop.com.acme.mark8ly.com`. This lets Mark8ly issue and renew a free Let's Encrypt certificate for your domain without you sharing DNS credentials.

**3. (Optional) Admin subdomain.** Add an `A` record at `admin.yourshop.com` pointing to the same IP if you want a branded admin URL. Skip if you're happy logging in at the default admin URL.

DNS changes can take up to 48 hours to propagate, though most registrars publish them within an hour. You can check propagation at [dnschecker.org](https://dnschecker.org). Once both required records are live, click **Verify** on the domain card.

## Automated setup with Cloudflare — API token guide

The Cloudflare (auto) method asks for a Cloudflare API token. Mark8ly uses the token only to add the routing CNAME on your zone — never to read DNS for any other zone, and never to make changes outside your domain. The token is stored in Google Cloud Secret Manager scoped to your tenant and is never written to a database or to logs.

### Required permissions on the token

The token needs **two** permissions:

- `Zone > DNS > Edit` — lets us add the CNAME record automatically.
- `Zone > Zone > Read` — lets us find your zone by domain name.

Under **Zone Resources**, scope the token to *only the zone for this domain*. Do not grant Account-level permissions or include other zones.

### Step-by-step — creating the token

1. Sign in to [dash.cloudflare.com → My Profile → API Tokens](https://dash.cloudflare.com/profile/api-tokens).
2. Click **Create Token** and pick the **Edit zone DNS** template.
3. Under *Zone Resources*, choose **Include → Specific zone → your domain**.
4. (Optional, recommended) Set an expiry date and add an IP allowlist for safety.
5. Click **Continue to summary**, then **Create Token**, and copy the token value. Cloudflare only shows it once.
6. Paste the token into the **Cloudflare API token** field on the Domains page and click **Add domain**.

### What happens after you submit

1. We securely store the token in Google Cloud Secret Manager under your tenant.
2. We look up your zone on Cloudflare and add a DNS-only CNAME pointing your domain to our edge.
3. The domain enters the **verifying** state while DNS propagates.
4. Once verified, we automatically issue a Let's Encrypt SSL certificate for the domain.

If something goes wrong, the domain card shows an inline error with a hint on what to check. The most common cause is a token without the right permissions or scoped to the wrong zone — re-create it and click **Verify** again.

### Revoking access

You can revoke the token at any time from your Cloudflare dashboard. If you remove the domain from Mark8ly, we delete the secret and the DNS record we created.

## SSL Certificates

Mark8ly automatically provisions and renews SSL certificates for your custom domain. Once DNS is properly configured, the certificate is issued within minutes. Your store will be accessible over HTTPS with a valid certificate, which is essential for customer trust and search engine rankings.

If the SSL certificate fails to provision, double-check your DNS records. The most common issue is a missing or incorrect CNAME record. The domain settings page shows the current status and any errors to help you troubleshoot.

## Multiple Domains

You can connect multiple custom domains to a single store. This is useful if you own several domain variations like `yourshop.com` and `yourshop.co.uk`. Set one domain as primary, which is the canonical URL used in emails and social shares. Additional domains redirect to the primary domain automatically.

## Domain Best Practices

Use a `www` subdomain as your primary domain rather than the bare root domain. This gives you more flexibility with DNS configuration and CDN routing. Most domain registrars support forwarding the bare domain to the `www` version.

Keep your domain registration current and enable auto-renewal to prevent accidental expiration. A lapsed domain can be registered by someone else, causing you to lose your store URL and any SEO authority you have built.
