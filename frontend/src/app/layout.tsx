import "./globals.css";

import type { Metadata, Viewport } from "next";
import type { ReactNode } from "react";

// Geist from the geist package (next/font/local): the font files ship in
// node_modules, so building the image needs no access to Google Fonts.
import { GeistMono } from "geist/font/mono";
import { GeistSans } from "geist/font/sans";

import { Providers } from "@/components/providers";
import { APPEARANCE_SCRIPT } from "@/lib/appearance-script";

export const metadata: Metadata = {
  title: { default: "Fleetdock", template: "%s · Fleetdock" },
  description: "Manage your databases, backups and servers",
  applicationName: "Fleetdock",
  openGraph: {
    title: "Fleetdock",
    description: "Manage your databases, backups and servers",
    siteName: "Fleetdock",
    type: "website",
  },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#ffffff" },
    { media: "(prefers-color-scheme: dark)", color: "#09090b" },
  ],
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html
      lang="en"
      className={`${GeistSans.variable} ${GeistMono.variable}`}
      // The appearance script sets .dark / data-style / data-borders before React hydrates.
      suppressHydrationWarning
    >
      <head>
        <script dangerouslySetInnerHTML={{ __html: APPEARANCE_SCRIPT }} />
      </head>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
