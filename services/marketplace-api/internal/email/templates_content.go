package email

// templates_content.go — embedded fallback copy for the twelve billing
// templates (#381).
//
// These are FALLBACKS. The authoritative copy, once an operator writes one,
// is the published row in email_templates; the loader prefers it and only
// reaches for these when the row is absent, draft, or the DB is unreachable.
// Keep them plain and provider-agnostic: no external images, no web fonts,
// table-based layout, inline styles only.
//
// Brand: paper (#F7F6F2) page, white card, ink (#0E0E0C) text, moss
// (#2D4A2B) for the single call to action. One accent, no decoration.

// chromeHTML wraps every fragment. <!--BODY--> is substituted at
// registration time; it is deliberately an HTML comment rather than a
// template action so the chrome itself is never parsed as a template.
const chromeHTML = `<!doctype html>
<html>
<body style="margin:0;padding:0;background:#F7F6F2;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#F7F6F2;">
<tr><td align="center" style="padding:40px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#FFFFFF;border-radius:6px;">
<tr><td style="padding:40px;font-family:'Source Sans 3',-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:16px;line-height:1.6;color:#0E0E0C;">
<!--BODY-->
<hr style="border:none;border-top:1px solid #E4E2DC;margin:32px 0 16px;">
<p style="font-size:12px;line-height:1.5;color:#6B6A64;margin:0;">Mark8ly · You are receiving this because you run a store on Mark8ly.</p>
</td></tr></table>
</td></tr></table>
</body>
</html>`

// h1 is the shared editorial headline style — Source Serif 4, per the
// brand direction that the serif carries the brand.
const h1 = `style="font-family:'Source Serif 4',Georgia,serif;font-size:26px;line-height:1.25;font-weight:600;margin:0 0 20px;"`

// cta renders the single moss button. Guarded by {{if}} at each call site
// so a missing URL omits the button rather than emitting an empty href.
const ctaOpen = `<p style="margin:28px 0 0;"><a href="`
const ctaMid = `" style="display:inline-block;background:#2D4A2B;color:#FFFFFF;text-decoration:none;padding:12px 22px;border-radius:6px;font-weight:600;">`
const ctaClose = `</a></p>`

// --- subjects -------------------------------------------------------

var billingSubjects = map[TemplateID]string{
	TemplateDunningDay5:           `We could not process your payment for {{.store_name}}`,
	TemplateDunningDay7:           `Action needed to keep {{.store_name}} open`,
	TemplatePaymentActionReminder: `Confirm your payment for {{.store_name}}`,
	TemplateTrialNoPMT15:          `15 days left in your {{.store_name}} trial`,
	TemplateTrialNoPMT10:          `10 days left in your {{.store_name}} trial`,
	TemplateTrialNoPMT7:           `One week left in your {{.store_name}} trial`,
	TemplateTrialNoPMT3:           `3 days left in your {{.store_name}} trial`,
	TemplateTrialNoPMT1:           `Your {{.store_name}} trial ends tomorrow`,
	TemplateTrialHasPMT1:          `Your {{.plan}} plan for {{.store_name}} starts tomorrow`,
	TemplateTrialStartedBilled:    `Your {{.plan}} plan for {{.store_name}} is active`,
	TemplateTrialExpired:          `Your {{.store_name}} trial has ended`,
	TemplateWinBack:               `Come back to Mark8ly — {{.percent_off}}% off for {{.duration_months}} months`,
	TemplateWinBackNoOffer:        `Your {{.store_name}} store is still here`,

	TemplateMigrationFastPathApproved: `Your migration request for {{.store_name}} is approved`,
	TemplateMigrationFastPathRejected: `We could not approve the migration request for {{.store_name}}`,
}

// --- HTML fragments -------------------------------------------------

var billingHTMLFragments = map[TemplateID]string{
	TemplateDunningDay5: `<h1 ` + h1 + `>Your payment did not go through</h1>
<p>We tried to charge the card on file for <strong>{{.store_name}}</strong> and it was declined. Your store is still open.</p>
<p>Updating your payment method now avoids any interruption.</p>
{{if .hosted_invoice_url}}` + ctaOpen + `{{.hosted_invoice_url}}` + ctaMid + `Complete your payment` + ctaClose + `{{else}}<p>Open your Mark8ly billing settings to update your card.</p>{{end}}`,

	TemplateDunningDay7: `<h1 ` + h1 + `>Action needed to keep {{.store_name}} open</h1>
<p>It has been {{.day}} days since your payment failed. If it stays unpaid, <strong>{{.store_name}}</strong> will be suspended and your storefront will stop serving customers.</p>
<p>This is reversible right up until suspension.</p>
{{if .hosted_invoice_url}}` + ctaOpen + `{{.hosted_invoice_url}}` + ctaMid + `Pay now` + ctaClose + `{{else}}<p>Open your Mark8ly billing settings to update your card.</p>{{end}}`,

	TemplatePaymentActionReminder: `<h1 ` + h1 + `>One step left to confirm your payment</h1>
<p>Your bank asked for extra confirmation before charging the card for <strong>{{.store_name}}</strong>. Until you approve it, the payment is not complete.</p>
{{if .hosted_invoice_url}}` + ctaOpen + `{{.hosted_invoice_url}}` + ctaMid + `Confirm payment` + ctaClose + `{{else}}<p>Open your Mark8ly billing settings to finish confirming.</p>{{end}}`,

	TemplateTrialNoPMT15: `<h1 ` + h1 + `>15 days left in your trial</h1>
<p><strong>{{.store_name}}</strong> has {{.days_remaining}} days left on the free trial. There is no card on file yet.</p>
<p>Adding one now means your storefront keeps serving customers the moment the trial ends. Nothing is charged until then.</p>`,

	TemplateTrialNoPMT10: `<h1 ` + h1 + `>10 days left in your trial</h1>
<p><strong>{{.store_name}}</strong> has {{.days_remaining}} days left on the free trial, and no payment method on file.</p>
<p>Choose a plan whenever you are ready — you will not be charged before the trial ends.</p>`,

	TemplateTrialNoPMT7: `<h1 ` + h1 + `>One week left in your trial</h1>
<p><strong>{{.store_name}}</strong> has {{.days_remaining}} days of trial remaining. There is still no card on file.</p>
<p>Without one, your storefront stops serving customers when the trial ends.</p>`,

	TemplateTrialNoPMT3: `<h1 ` + h1 + `>3 days left in your trial</h1>
<p><strong>{{.store_name}}</strong> has {{.days_remaining}} days left. Adding a payment method takes a minute and keeps everything running.</p>`,

	TemplateTrialNoPMT1: `<h1 ` + h1 + `>Your trial ends tomorrow</h1>
<p>Tomorrow the free trial for <strong>{{.store_name}}</strong> ends. With no payment method on file, the storefront will stop serving customers.</p>
<p>Your products, orders and settings are kept — adding a card restores the store immediately.</p>`,

	TemplateTrialHasPMT1: `<h1 ` + h1 + `>Your {{.plan}} plan starts tomorrow</h1>
<p>The free trial for <strong>{{.store_name}}</strong> ends tomorrow, and your <strong>{{.plan}}</strong> plan begins. We will charge the card on file — nothing for you to do.</p>
<p>If you would rather change plan, you can do that before the charge.</p>`,

	TemplateTrialStartedBilled: `<h1 ` + h1 + `>Your {{.plan}} plan is active</h1>
<p>Thank you — the first payment for <strong>{{.store_name}}</strong> went through and your <strong>{{.plan}}</strong> plan is now active, billed {{.period}}.</p>
<p>Your receipt is in your billing settings.</p>`,

	TemplateTrialExpired: `<h1 ` + h1 + `>Your trial has ended</h1>
<p>The free trial for <strong>{{.store_name}}</strong> ended today. No payment method was added, so the store is not on a paid plan and the storefront has stopped serving customers.</p>
<p>Nothing has been deleted. Your products, orders and settings are exactly as you left them.</p>
<p>To reopen the store, add a payment method and choose a plan in your Mark8ly billing settings — it comes back straight away.</p>`,

	// The discount sentence names {{.promo_code}}, so this fragment cannot
	// render a credible offer without one — the cron only reaches this key
	// when it holds a code that validated. Its numbers come from the promo
	// row too, so a console edit to the discount cannot leave the email
	// quoting a stale one.
	//
	// It deliberately does NOT tell the merchant where to enter the code.
	// POST /admin/stores/:storeId/subscription/apply-promo has no caller in
	// apps/ — there is no field in the merchant admin that redeems a promo
	// code — so naming a screen would be the same class of untrue statement
	// this template was fixed to stop making. Whoever authors the code in
	// the console owns closing that gap first.
	TemplateWinBack: `<h1 ` + h1 + `>Your store is still here</h1>
<p><strong>{{.store_name}}</strong> has been closed for a month. Everything — products, orders, settings — is exactly as you left it.</p>
<p>If you want to pick it back up, use the code <strong>{{.promo_code}}</strong> when you reopen your store and we will take <strong>{{.percent_off}}% off your first {{.duration_months}} months</strong>.</p>`,

	// The offer-less variant. It carries no number and names no code,
	// because when this key is chosen there is nothing to name. What is
	// left is still worth sending: a merchant a month past expiry does not
	// know their catalogue survived.
	TemplateWinBackNoOffer: `<h1 ` + h1 + `>Your store is still here</h1>
<p><strong>{{.store_name}}</strong> has been closed for a month. Everything — products, orders, settings — is exactly as you left it.</p>
<p>Nothing has been deleted. Whenever you want to reopen it, add a payment method and choose a plan in your Mark8ly billing settings — the storefront comes back straight away.</p>`,

	TemplateMigrationFastPathApproved: `<h1 ` + h1 + `>Your migration request is approved</h1>
<p>We reviewed the evidence you sent for <strong>{{.store_name}}</strong> and approved your migration request. Nothing further is needed from you.</p>
<p>You can see the current state of your plan any time in your Mark8ly billing settings.</p>`,

	TemplateMigrationFastPathRejected: `<h1 ` + h1 + `>We could not approve your migration request</h1>
<p>We reviewed the evidence you sent for <strong>{{.store_name}}</strong> and were not able to approve the migration request this time.</p>
<p>Your store is unaffected — this changes nothing about your current plan, and you can submit a new request with different evidence.</p>
<p>If you would like to know more about this decision, reply to this email and our team will go through it with you.</p>`,
}

// --- text fragments -------------------------------------------------

var billingTextFragments = map[TemplateID]string{
	TemplateDunningDay5: `Your payment did not go through

We tried to charge the card on file for {{.store_name}} and it was declined. Your store is still open.

Updating your payment method now avoids any interruption.
{{if .hosted_invoice_url}}
Complete your payment: {{.hosted_invoice_url}}
{{else}}
Open your Mark8ly billing settings to update your card.
{{end}}
Mark8ly`,

	TemplateDunningDay7: `Action needed to keep {{.store_name}} open

It has been {{.day}} days since your payment failed. If it stays unpaid, {{.store_name}} will be suspended and your storefront will stop serving customers.

This is reversible right up until suspension.
{{if .hosted_invoice_url}}
Pay now: {{.hosted_invoice_url}}
{{else}}
Open your Mark8ly billing settings to update your card.
{{end}}
Mark8ly`,

	TemplatePaymentActionReminder: `One step left to confirm your payment

Your bank asked for extra confirmation before charging the card for {{.store_name}}. Until you approve it, the payment is not complete.
{{if .hosted_invoice_url}}
Confirm payment: {{.hosted_invoice_url}}
{{else}}
Open your Mark8ly billing settings to finish confirming.
{{end}}
Mark8ly`,

	TemplateTrialNoPMT15: `15 days left in your trial

{{.store_name}} has {{.days_remaining}} days left on the free trial. There is no card on file yet.

Adding one now means your storefront keeps serving customers the moment the trial ends. Nothing is charged until then.

Mark8ly`,

	TemplateTrialNoPMT10: `10 days left in your trial

{{.store_name}} has {{.days_remaining}} days left on the free trial, and no payment method on file.

Choose a plan whenever you are ready — you will not be charged before the trial ends.

Mark8ly`,

	TemplateTrialNoPMT7: `One week left in your trial

{{.store_name}} has {{.days_remaining}} days of trial remaining. There is still no card on file.

Without one, your storefront stops serving customers when the trial ends.

Mark8ly`,

	TemplateTrialNoPMT3: `3 days left in your trial

{{.store_name}} has {{.days_remaining}} days left. Adding a payment method takes a minute and keeps everything running.

Mark8ly`,

	TemplateTrialNoPMT1: `Your trial ends tomorrow

Tomorrow the free trial for {{.store_name}} ends. With no payment method on file, the storefront will stop serving customers.

Your products, orders and settings are kept — adding a card restores the store immediately.

Mark8ly`,

	TemplateTrialHasPMT1: `Your {{.plan}} plan starts tomorrow

The free trial for {{.store_name}} ends tomorrow, and your {{.plan}} plan begins. We will charge the card on file — nothing for you to do.

If you would rather change plan, you can do that before the charge.

Mark8ly`,

	TemplateTrialStartedBilled: `Your {{.plan}} plan is active

Thank you — the first payment for {{.store_name}} went through and your {{.plan}} plan is now active, billed {{.period}}.

Your receipt is in your billing settings.

Mark8ly`,

	TemplateTrialExpired: `Your trial has ended

The free trial for {{.store_name}} ended today. No payment method was added, so the store is not on a paid plan and the storefront has stopped serving customers.

Nothing has been deleted. Your products, orders and settings are exactly as you left them.

To reopen the store, add a payment method and choose a plan in your Mark8ly billing settings — it comes back straight away.

Mark8ly`,

	TemplateWinBack: `Your store is still here

{{.store_name}} has been closed for a month. Everything — products, orders, settings — is exactly as you left it.

If you want to pick it back up, use the code {{.promo_code}} when you reopen your store and we will take {{.percent_off}}% off your first {{.duration_months}} months.

Mark8ly`,

	TemplateWinBackNoOffer: `Your store is still here

{{.store_name}} has been closed for a month. Everything — products, orders, settings — is exactly as you left it.

Nothing has been deleted. Whenever you want to reopen it, add a payment method and choose a plan in your Mark8ly billing settings — the storefront comes back straight away.

Mark8ly`,

	TemplateMigrationFastPathApproved: `Your migration request is approved

We reviewed the evidence you sent for {{.store_name}} and approved your migration request. Nothing further is needed from you.

You can see the current state of your plan any time in your Mark8ly billing settings.

Mark8ly`,

	TemplateMigrationFastPathRejected: `We could not approve your migration request

We reviewed the evidence you sent for {{.store_name}} and were not able to approve the migration request this time.

Your store is unaffected — this changes nothing about your current plan, and you can submit a new request with different evidence.

If you would like to know more about this decision, reply to this email and our team will go through it with you.

Mark8ly`,
}
