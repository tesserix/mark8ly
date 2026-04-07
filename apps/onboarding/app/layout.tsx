import type { ReactNode } from "react";

export const metadata = {
  title: "Mark8ly Onboarding",
  description: "Get your store live on Mark8ly.",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
