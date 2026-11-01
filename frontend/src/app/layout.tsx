import "./globals.css";

import type { Metadata, Viewport } from "next";
import type { ReactNode } from "react";

import { Geist, Geist_Mono } from "next/font/google";

import { Providers } from "@/components/providers";

const sans = Geist({
  subsets: ["latin"],
  variable: "--font-geist-sans",
  display: "swap",
});
const mono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-geist-mono",
  display: "swap",
});

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
    { media: "(prefers-color-scheme: light)", color: "#0f766e" },
    { media: "(prefers-color-scheme: dark)", color: "#09090b" },
  ],
};

// Applies the persisted theme before paint to avoid a flash of the wrong theme.
const themeScript = `(function(){try{var t=localStorage.getItem('theme');var d=t?t==='dark':window.matchMedia('(prefers-color-scheme: dark)').matches;if(d)document.documentElement.classList.add('dark');}catch(e){}})();`;

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html
      lang="en"
      className={`${sans.variable} ${mono.variable}`}
      suppressHydrationWarning
    >
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
