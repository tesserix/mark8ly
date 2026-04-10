---
title: "Custom Domains"
category: "Store Setup"
order: 11
---

## Default Domain

Every Mark8ly store comes with a free subdomain at `yourstore.mark8ly.com`. This domain is active immediately when you create your store and requires no configuration. It is a good starting point while you set up the rest of your store.

The default subdomain uses HTTPS automatically and is backed by Mark8ly's content delivery network for fast loading anywhere in the world.

## Connecting a Custom Domain

To use your own domain like `www.yourshop.com`, navigate to **Settings > Domains** and enter your domain name. Mark8ly will provide DNS records that you need to add at your domain registrar.

Typically you need to create a CNAME record pointing your domain to Mark8ly's routing infrastructure. The exact records are displayed in the domain settings page after you add your domain. DNS changes can take up to forty-eight hours to propagate, though most registrars process them within a few hours.

## SSL Certificates

Mark8ly automatically provisions and renews SSL certificates for your custom domain. Once DNS is properly configured, the certificate is issued within minutes. Your store will be accessible over HTTPS with a valid certificate, which is essential for customer trust and search engine rankings.

If the SSL certificate fails to provision, double-check your DNS records. The most common issue is a missing or incorrect CNAME record. The domain settings page shows the current status and any errors to help you troubleshoot.

## Multiple Domains

You can connect multiple custom domains to a single store. This is useful if you own several domain variations like `yourshop.com` and `yourshop.co.uk`. Set one domain as primary, which is the canonical URL used in emails and social shares. Additional domains redirect to the primary domain automatically.

## Domain Best Practices

Use a `www` subdomain as your primary domain rather than the bare root domain. This gives you more flexibility with DNS configuration and CDN routing. Most domain registrars support forwarding the bare domain to the `www` version.

Keep your domain registration current and enable auto-renewal to prevent accidental expiration. A lapsed domain can be registered by someone else, causing you to lose your store URL and any SEO authority you have built.
