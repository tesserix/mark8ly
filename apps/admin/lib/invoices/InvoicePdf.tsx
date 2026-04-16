// InvoicePdf / ReceiptPdf — react-pdf templates.
//
// Single layout, two title variants, switched on `doc.kind`:
//   • invoice → "INVOICE" + "Amount due"
//   • receipt → "RECEIPT" + "Paid in full"
//
// Designed to look like a proper business document: serif title, sans-
// serif body, hairline rules, tabular figures, generous whitespace. No
// decorative blocks. The brand accent colour appears only as a thin top
// rule and on the totals divider; everything else is greyscale so it
// prints cleanly on B&W and in colour.

import { Document, Page, View, Text, Image, StyleSheet } from "@react-pdf/renderer";

import type {
  DocumentAddress,
  DocumentLine,
  DocumentTotals,
  InvoiceDocument,
} from "./types";

// We use react-pdf's built-in fonts (Helvetica for body, Times-Roman for
// the document title) so no external network fetch is required at render
// time. This makes the PDF endpoint resilient — egress restrictions or a
// fonts.gstatic.com outage cannot break invoice generation.
const SANS = "Helvetica";
const SANS_BOLD = "Helvetica-Bold";
const SERIF = "Times-Roman";
const SERIF_BOLD = "Times-Bold";

const INK = "#0E0E0C";
const PAPER = "#FFFFFF";
const SUBDUED = "#5A5A55";
const HAIRLINE = "#D9D7CF";
const SUBTLE_FILL = "#F7F6F2";

// ─────────────────────────────────────────────────────────────────────────
// Styles
// ─────────────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  page: {
    paddingTop: 48,
    paddingBottom: 56,
    paddingHorizontal: 48,
    fontFamily: SANS,
    fontSize: 9.5,
    color: INK,
    backgroundColor: PAPER,
    lineHeight: 1.5,
  },

  accentRule: { height: 3, marginBottom: 24 },

  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-start",
    marginBottom: 28,
  },
  logo: { maxWidth: 130, maxHeight: 52, objectFit: "contain" },
  logoFallback: { fontFamily: SERIF_BOLD, fontSize: 22, color: INK, letterSpacing: 0.2 },
  storeMeta: { textAlign: "right", flexDirection: "column", gap: 2 },
  storeName: { fontFamily: SERIF_BOLD, fontSize: 11, color: INK },
  storeMetaLine: { fontSize: 8.5, color: SUBDUED },

  titleBlock: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-end",
    marginBottom: 28,
    paddingBottom: 18,
    borderBottomWidth: 0.5,
    borderBottomColor: HAIRLINE,
    borderBottomStyle: "solid",
  },
  titleHeading: { fontFamily: SERIF_BOLD, fontSize: 28, letterSpacing: 4, color: INK },
  titleMeta: { flexDirection: "column", alignItems: "flex-end", gap: 2 },
  metaLabel: {
    fontFamily: SANS_BOLD,
    fontSize: 7.5,
    letterSpacing: 1.2,
    textTransform: "uppercase",
    color: SUBDUED,
  },
  metaValue: { fontSize: 11, color: INK },

  partiesRow: { flexDirection: "row", gap: 32, marginBottom: 28 },
  partyBlock: { flex: 1, flexDirection: "column", gap: 4 },
  partyLabel: {
    fontFamily: SANS_BOLD,
    fontSize: 7.5,
    letterSpacing: 1.2,
    textTransform: "uppercase",
    color: SUBDUED,
    marginBottom: 6,
  },
  partyName: { fontFamily: SANS_BOLD, fontSize: 10.5, color: INK },
  partyLine: { fontSize: 9.5, color: INK, lineHeight: 1.4 },
  partyEmail: { fontSize: 9, color: SUBDUED, marginTop: 2 },

  table: { flexDirection: "column", marginBottom: 22 },
  tableHeader: {
    flexDirection: "row",
    paddingVertical: 8,
    borderBottomWidth: 1,
    borderBottomColor: INK,
    borderBottomStyle: "solid",
  },
  tableRow: {
    flexDirection: "row",
    paddingVertical: 10,
    borderBottomWidth: 0.5,
    borderBottomColor: HAIRLINE,
    borderBottomStyle: "solid",
  },
  th: {
    fontFamily: SANS_BOLD,
    fontSize: 7.5,
    letterSpacing: 1.1,
    textTransform: "uppercase",
    color: SUBDUED,
  },
  td: { fontSize: 9.5, color: INK },
  tdMuted: { fontSize: 8.5, color: SUBDUED, marginTop: 1 },
  colDescription: { flex: 5, paddingRight: 10 },
  colQty: { flex: 1, textAlign: "right", paddingRight: 8 },
  colPrice: { flex: 1.6, textAlign: "right", paddingRight: 8 },
  colTotal: { flex: 1.6, textAlign: "right" },

  totalsRow: { flexDirection: "row", justifyContent: "flex-end", marginBottom: 28 },
  totalsBox: { width: 240, flexDirection: "column", gap: 6 },
  totalsLine: { flexDirection: "row", justifyContent: "space-between" },
  totalsLabel: { fontSize: 9.5, color: SUBDUED },
  totalsValue: { fontSize: 9.5, color: INK },
  grandRule: { marginVertical: 6, height: 1 },
  grandLine: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  grandLabel: { fontFamily: SANS_BOLD, fontSize: 11, color: INK },
  grandValue: { fontFamily: SERIF_BOLD, fontSize: 16, color: INK },

  statusBanner: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: 10,
    borderTopWidth: 0.5,
    borderTopColor: HAIRLINE,
    borderTopStyle: "solid",
    borderBottomWidth: 0.5,
    borderBottomColor: HAIRLINE,
    borderBottomStyle: "solid",
    marginBottom: 24,
  },
  statusLabel: {
    fontFamily: SERIF_BOLD,
    fontSize: 12,
    letterSpacing: 1.4,
    textTransform: "uppercase",
  },
  statusMeta: { fontSize: 8.5, color: SUBDUED },

  notes: {
    backgroundColor: SUBTLE_FILL,
    padding: 12,
    fontSize: 8.5,
    color: SUBDUED,
    marginBottom: 24,
    lineHeight: 1.55,
  },

  footer: {
    position: "absolute",
    bottom: 32,
    left: 48,
    right: 48,
    paddingTop: 10,
    borderTopWidth: 0.5,
    borderTopColor: HAIRLINE,
    borderTopStyle: "solid",
    flexDirection: "row",
    justifyContent: "space-between",
    fontSize: 7.5,
    color: SUBDUED,
  },
  pageNum: { fontSize: 7.5, color: SUBDUED },
});

// ─────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────

function formatCurrency(amount: string | number, currency: string): string {
  const n = typeof amount === "string" ? Number.parseFloat(amount) : amount;
  if (Number.isNaN(n)) return `${currency} ${amount}`;
  try {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(n);
  } catch {
    return `${currency} ${n.toFixed(2)}`;
  }
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
    });
  } catch {
    return iso;
  }
}

function isPaid(status: string): boolean {
  return status === "paid" || status === "captured" || status === "partially_refunded";
}

function ItemRow({ line, currency }: { line: DocumentLine; currency: string }) {
  return (
    <View style={styles.tableRow} wrap={false}>
      <View style={styles.colDescription}>
        <Text style={styles.td}>{line.description}</Text>
        {line.sku && <Text style={styles.tdMuted}>{line.sku}</Text>}
      </View>
      <Text style={[styles.td, styles.colQty]}>{line.quantity}</Text>
      <Text style={[styles.td, styles.colPrice]}>{formatCurrency(line.unit_price, currency)}</Text>
      <Text style={[styles.td, styles.colTotal]}>{formatCurrency(line.line_total, currency)}</Text>
    </View>
  );
}

function PartyBlock({
  label,
  address,
  email,
}: {
  label: string;
  address: DocumentAddress;
  email?: string;
}) {
  return (
    <View style={styles.partyBlock}>
      <Text style={styles.partyLabel}>{label}</Text>
      <Text style={styles.partyName}>{address.name}</Text>
      <Text style={styles.partyLine}>{address.line1}</Text>
      {address.line2 ? <Text style={styles.partyLine}>{address.line2}</Text> : null}
      <Text style={styles.partyLine}>
        {address.city}
        {address.region ? `, ${address.region}` : ""}
        {address.postal_code ? ` ${address.postal_code}` : ""}
      </Text>
      <Text style={styles.partyLine}>{address.country_code}</Text>
      {address.phone ? <Text style={styles.partyLine}>{address.phone}</Text> : null}
      {email ? <Text style={styles.partyEmail}>{email}</Text> : null}
    </View>
  );
}

function Totals({ totals }: { totals: DocumentTotals }) {
  const refunded = totals.refunded_amount ? Number.parseFloat(totals.refunded_amount) : 0;
  const ccy = totals.currency_code;
  return (
    <View style={styles.totalsRow}>
      <View style={styles.totalsBox}>
        <View style={styles.totalsLine}>
          <Text style={styles.totalsLabel}>Subtotal</Text>
          <Text style={styles.totalsValue}>{formatCurrency(totals.subtotal, ccy)}</Text>
        </View>
        <View style={styles.totalsLine}>
          <Text style={styles.totalsLabel}>Shipping</Text>
          <Text style={styles.totalsValue}>{formatCurrency(totals.shipping_total, ccy)}</Text>
        </View>
        <View style={styles.totalsLine}>
          <Text style={styles.totalsLabel}>Tax</Text>
          <Text style={styles.totalsValue}>{formatCurrency(totals.tax_total, ccy)}</Text>
        </View>
        {Number.parseFloat(totals.discount_total) > 0 && (
          <View style={styles.totalsLine}>
            <Text style={styles.totalsLabel}>Discount</Text>
            <Text style={styles.totalsValue}>−{formatCurrency(totals.discount_total, ccy)}</Text>
          </View>
        )}
        <View style={[styles.grandRule, { backgroundColor: INK }]} />
        <View style={styles.grandLine}>
          <Text style={styles.grandLabel}>Total</Text>
          <Text style={styles.grandValue}>{formatCurrency(totals.grand_total, ccy)}</Text>
        </View>
        {refunded > 0 && (
          <View style={[styles.totalsLine, { marginTop: 6 }]}>
            <Text style={styles.totalsLabel}>Refunded</Text>
            <Text style={styles.totalsValue}>−{formatCurrency(refunded, ccy)}</Text>
          </View>
        )}
      </View>
    </View>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Document
// ─────────────────────────────────────────────────────────────────────────

interface PdfProps {
  doc: InvoiceDocument;
}

export function InvoicePdf({ doc }: PdfProps) {
  const accent = doc.store.color_accent || INK;
  const isReceipt = doc.kind === "receipt";
  const titleText = isReceipt ? "RECEIPT" : "INVOICE";
  // Invoice statuses pivot on whether the customer has settled payment.
  // Receipts are only ever issued post-delivery, so the banner reads as
  // a confirmation rather than a request.
  const statusLabel = isReceipt
    ? "DELIVERED"
    : isPaid(doc.payment_status)
      ? "PAID"
      : "AMOUNT DUE";
  const statusMeta = isReceipt
    ? `Order delivered ${doc.payment_date ? formatDate(doc.payment_date) : formatDate(doc.issue_date)}`
    : isPaid(doc.payment_status)
      ? `Payment received ${formatDate(doc.issue_date)}`
      : `Please remit ${formatCurrency(doc.totals.grand_total, doc.totals.currency_code)} on or before this date`;

  const ship = doc.ship_to ?? doc.bill_to;
  const showShipBlock = doc.ship_to && JSON.stringify(doc.ship_to) !== JSON.stringify(doc.bill_to);

  return (
    <Document
      title={`${titleText} ${doc.document_number}`}
      author={doc.store.name}
      subject={`${titleText} for ${doc.order_number}`}
      creator={doc.store.name}
      producer={doc.store.name}
    >
      <Page size="A4" style={styles.page}>
        <View style={[styles.accentRule, { backgroundColor: accent }]} fixed />

        {/* Header — logo + store identity */}
        <View style={styles.header}>
          {doc.store.logo_url ? (
            <Image style={styles.logo} src={doc.store.logo_url} />
          ) : (
            <Text style={styles.logoFallback}>{doc.store.name}</Text>
          )}
          <View style={styles.storeMeta}>
            <Text style={styles.storeName}>{doc.store.name}</Text>
            {doc.store.tagline ? <Text style={styles.storeMetaLine}>{doc.store.tagline}</Text> : null}
            {doc.store.contact_email ? <Text style={styles.storeMetaLine}>{doc.store.contact_email}</Text> : null}
          </View>
        </View>

        {/* Title + meta */}
        <View style={styles.titleBlock}>
          <Text style={styles.titleHeading}>{titleText}</Text>
          <View style={styles.titleMeta}>
            <Text style={styles.metaLabel}>{titleText} №</Text>
            <Text style={styles.metaValue}>{doc.document_number}</Text>
            <Text style={[styles.metaLabel, { marginTop: 8 }]}>Issued</Text>
            <Text style={styles.metaValue}>{formatDate(doc.issue_date)}</Text>
            <Text style={[styles.metaLabel, { marginTop: 8 }]}>Order</Text>
            <Text style={styles.metaValue}>{doc.order_number}</Text>
          </View>
        </View>

        {/* Parties */}
        <View style={styles.partiesRow}>
          <PartyBlock label="Bill to" address={doc.bill_to} email={doc.customer_email} />
          {showShipBlock && <PartyBlock label="Ship to" address={ship} />}
        </View>

        {/* Line items */}
        <View style={styles.table}>
          <View style={styles.tableHeader}>
            <Text style={[styles.th, styles.colDescription]}>Description</Text>
            <Text style={[styles.th, styles.colQty]}>Qty</Text>
            <Text style={[styles.th, styles.colPrice]}>Unit price</Text>
            <Text style={[styles.th, styles.colTotal]}>Amount</Text>
          </View>
          {doc.lines.map((line, i) => (
            <ItemRow key={i} line={line} currency={doc.totals.currency_code} />
          ))}
        </View>

        {/* Totals */}
        <Totals totals={doc.totals} />

        {/* Status banner */}
        <View style={styles.statusBanner}>
          <Text style={[styles.statusLabel, { color: isReceipt || isPaid(doc.payment_status) ? accent : INK }]}>
            {statusLabel}
          </Text>
          <Text style={styles.statusMeta}>{statusMeta}</Text>
        </View>

        {/* Notes */}
        {doc.notes ? <View style={styles.notes}><Text>{doc.notes}</Text></View> : null}

        {/* Footer (fixed on every page) */}
        <View style={styles.footer} fixed>
          <Text>
            {doc.store.name}
            {doc.store.contact_email ? ` · ${doc.store.contact_email}` : ""}
          </Text>
          <Text
            style={styles.pageNum}
            render={({ pageNumber, totalPages }) => `Page ${pageNumber} of ${totalPages}`}
          />
        </View>
      </Page>
    </Document>
  );
}

// Convenience re-export for clarity at call sites: identical layout, the
// `kind` field on the document drives the title + status copy.
export const ReceiptPdf = InvoicePdf;
