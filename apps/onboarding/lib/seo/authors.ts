/**
 * Named article authors.
 *
 * Most pages here publish under the Organization, which is honest for
 * product documentation and fine for it. A named byline is reserved for
 * guides making claims that need a person behind them — and it is only
 * worth adding where the person's *stated* experience actually covers
 * the topic (tesserix/mark8ly#594).
 *
 * That constraint is the whole point. A byline invites the reader to
 * check, so a name attached to a subject the bio does not support is
 * worse than no name at all: the reader clicks through and finds the
 * mismatch. Every `bio` here must describe work the person did, on the
 * subject at hand, and `sameAs` must point at third-party-hosted
 * profiles so the claim is verifiable somewhere we do not control.
 */

export interface Author {
  /**
   * Stable `@id` for the Person node, anchored at /about where the bio
   * is also rendered — so the identifier resolves to something a reader
   * and a crawler can both actually fetch.
   */
  id: string;
  name: string;
  jobTitle: string;
  /** Rendered under the byline. Must be true and topic-relevant. */
  bio: string;
  /** Third-party profiles. This is what makes the byline checkable. */
  sameAs: ReadonlyArray<string>;
}

export const MAHESH_SANGAWAR: Author = {
  id: "https://mark8ly.com/about#mahesh-sangawar",
  name: "Mahesh Sangawar",
  jobTitle: "Co-founder, Tesserix",
  bio: "Co-founder of Tesserix, the studio behind Mark8ly, and the principal author of Mark8ly's payment integration — including the Razorpay checkout that handles UPI, cards and wallets for merchants selling in India.",
  sameAs: [
    "https://www.linkedin.com/in/mahesh-sangawar-985a3214/",
    "https://github.com/mahesh-sangawar",
    "https://tesserix.app/about",
  ],
};
