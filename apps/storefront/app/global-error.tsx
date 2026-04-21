"use client";

interface GlobalErrorProps {
  error: Error & { digest?: string };
  reset: () => void;
}

export default function GlobalError({ error, reset }: GlobalErrorProps) {
  return (
    <html lang="en">
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <title>Something went wrong</title>
        <style>{`
          /* Global-error runs outside RootLayout — the merchant's
             theme CSS vars are not available here, so we intentionally
             use a tenant-neutral palette (paper / ink) that reads well
             on any brand and never looks like foreign matter glued on.
             No coloured accent — neutrals only. */
          @import url('https://fonts.googleapis.com/css2?family=Source+Serif+4:wght@400;500&display=swap');
          *, *::before, *::after { box-sizing: border-box; margin: 0; }
          body {
            font-family: 'Source Sans 3', system-ui, -apple-system, sans-serif;
            background: #F7F6F2;
            color: #0E0E0C;
            -webkit-font-smoothing: antialiased;
          }
          .wrap {
            display: grid;
            min-height: 100vh;
            grid-template-rows: 1fr auto;
          }
          .main {
            display: flex;
            align-items: flex-end;
            padding: 6rem 1.5rem 4rem;
          }
          @media (min-width: 640px) { .main { padding-left: 3rem; padding-right: 3rem; } }
          @media (min-width: 1024px) { .main { padding-left: 6rem; padding-right: 6rem; } }
          .content { max-width: 36rem; }
          .eyebrow {
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-size: 11px;
            font-weight: 500;
            text-transform: uppercase;
            letter-spacing: 0.2em;
            color: rgba(14, 14, 12, 0.65);
          }
          .rule {
            width: 48px;
            height: 1px;
            background: rgba(14, 14, 12, 0.2);
            margin-top: 6px;
          }
          .heading {
            margin-top: 1.5rem;
            font-family: 'Source Serif 4', Georgia, serif;
            font-size: clamp(2rem, 5vw, 3.5rem);
            font-weight: 500;
            line-height: 1.1;
            letter-spacing: -0.025em;
          }
          .desc {
            margin-top: 1.5rem;
            max-width: 28rem;
            font-size: 15px;
            line-height: 1.7;
            color: rgba(14, 14, 12, 0.7);
          }
          .ref {
            margin-top: 1rem;
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-size: 11px;
            color: rgba(14, 14, 12, 0.45);
          }
          .btn {
            margin-top: 1.5rem;
            display: inline-flex;
            height: 40px;
            align-items: center;
            padding: 0 20px;
            border-radius: 6px;
            border: none;
            background: #0E0E0C;
            color: #F7F6F2;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: background 200ms;
          }
          .btn:hover { background: #1a1a18; }
          .btn:focus-visible {
            outline: 2px solid #0E0E0C;
            outline-offset: 2px;
          }
          .footer {
            border-top: 1px solid rgba(14, 14, 12, 0.08);
            padding: 1.25rem 1.5rem;
          }
          @media (min-width: 640px) { .footer { padding-left: 3rem; padding-right: 3rem; } }
          .footer-text { font-size: 12px; color: rgba(14, 14, 12, 0.45); }
        `}</style>
      </head>
      <body>
        <div className="wrap">
          <main className="main">
            <div className="content">
              <p className="eyebrow">Something went wrong</p>
              <div className="rule" />
              <h1 className="heading">We hit a snag</h1>
              <p className="desc">
                Something unexpected happened. Please try refreshing the page.
                If this keeps happening, the store team has been notified.
              </p>
              {error.digest && (
                <p className="ref">ref&ensp;{error.digest}</p>
              )}
              <button type="button" onClick={reset} className="btn">
                Try again
              </button>
            </div>
          </main>
          <footer className="footer">
            <p className="footer-text">Powered by mark8ly</p>
          </footer>
        </div>
      </body>
    </html>
  );
}
