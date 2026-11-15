"use client";

import "./globals.css";

import { useEffect } from "react";

import { buttonClass } from "@/components/ui";

// The last resort: the root layout itself failed, so this replaces the whole
// document and must bring its own <html>, styles and the .dark class.
export default function GlobalError({ error, retry }: { error: Error & { digest?: string }; retry: () => void }) {
  useEffect(() => {
    console.error(error);
  }, [error]);
  return (
    <html lang="en" className="dark">
      <body>
        <title>Fleetdock hit an error</title>
        <main className="standalone-page">
          <h1 className="text-xl font-semibold m-0">Fleetdock hit an unexpected error</h1>
          <p className="muted mt-2 mb-4">
            Reload to try again. If it keeps happening, check the Fleetdock logs
            {error.digest ? (
              <>
                {" "}
                (reference <span className="mono">{error.digest}</span>)
              </>
            ) : null}
            .
          </p>
          <div className="flex gap-2 justify-center">
            <button type="button" className={buttonClass({ variant: "primary" })} onClick={() => retry()}>
              Try again
            </button>
            <a className={buttonClass()} href="/dashboard">
              Go to overview
            </a>
          </div>
        </main>
      </body>
    </html>
  );
}
