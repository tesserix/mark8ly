---
title: "Tax Configuration"
category: "Store Setup"
order: 5
---

Mark8ly calculates tax automatically at checkout based on the rules you configure under **Settings → Tax**. Rules are defined per region and applied as either **inclusive** or **exclusive** of the displayed product price.

> Tax compliance is ultimately your responsibility as the merchant. Mark8ly provides the calculation tools and reporting data — consult a tax professional to confirm your configuration meets local requirements.

## Inclusive vs. exclusive pricing

The right model depends on your jurisdiction:

| Model | How it works | Common in |
| --- | --- | --- |
| **Tax-inclusive** | Listed price already includes the tax. The tax is computed backward from the displayed amount. | EU (VAT), UK, Australia |
| **Tax-exclusive** | Tax is added on top of the listed price at checkout. | United States, Canada |

**Worked example.** A product listed at $100 with a 20% rate:

- **Inclusive:** customer pays $100. Base price = $83.33, tax = $16.67.
- **Exclusive:** customer pays $120. Base price = $100, tax = $20.

Choose the model that matches your local regulations and customer expectations.

## Creating tax rates

For each tax rate, you'll specify:

- **Region** — country or state.
- **Rate** — the percentage.
- **Applies to shipping** — yes or no, depending on local rules.

You can create multiple rates for different jurisdictions. For example, a US merchant might create separate rates for each state where they have a sales-tax obligation. Mark8ly applies the correct rate at checkout based on the customer's shipping address.

## Tax exemptions

Some products and customers can be exempt from tax.

- **Product-level** — mark specific products as tax-exempt if they fall into categories your jurisdiction exempts (e.g. groceries, medical supplies, educational materials).
- **Customer-level** — for B2B sales where the buyer provides a valid tax ID, apply a tax exemption to that customer.

Document your exemption policy clearly and keep records of any tax IDs you collect for compliance purposes.

## Reporting and compliance

Every order records its tax breakdown by rate and region. To file your returns:

1. Open **Orders** and filter by the period you're reporting.
2. Export the order list as CSV.
3. Reconcile the tax columns (`tax_amount`, `tax_rate`, `region`) against your filing.

The export includes everything you'll typically need: order total, tax amount, rate applied, and customer shipping address.
