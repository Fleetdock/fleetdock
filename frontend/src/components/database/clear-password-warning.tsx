import { Callout } from "@/components/ui";
import { sendsPasswordInClear } from "@/lib/engines";
import type { TLSMode } from "@/lib/types";

/**
 * ClearPasswordWarning warns, under the Encryption field, when the chosen
 * setting would send the admin password unencrypted (ClickHouse over plain
 * HTTP, which cannot fall back to TLS the way MySQL and PostgreSQL can).
 */
export function ClearPasswordWarning({ engine, tls, port }: { engine: string; tls: TLSMode; port: number }) {
  if (!sendsPasswordInClear(engine, tls, port)) return null;
  return (
    <Callout tone="warning">
      With this setting the password and all data travel unencrypted over HTTP. Choose “Always encrypt” and the
      server&apos;s HTTPS port (usually 8443) unless the network between Fleetdock and this server is trusted.
    </Callout>
  );
}
