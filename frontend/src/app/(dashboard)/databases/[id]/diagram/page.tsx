"use client";

import { ArrowLeft, Maximize, Minimize, Network, Table2 } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";

import { ERDiagram } from "@/components/database/er-diagram";
import { ThemeToggle } from "@/components/theme-toggle";
import { ButtonLink, EmptyState, IconButton, QueryError, Skeleton } from "@/components/ui";
import { useBrowserFullscreen, useFullscreenSupported } from "@/lib/focus-mode";
import { useDatabase } from "@/lib/hooks";

/**
 * The ER diagram of one database, using the whole window: the app's sidebar
 * and top bar are hidden on this route (see lib/focus-mode.ts), and a slim
 * bar leads back to the database.
 */
export default function DatabaseDiagramPage() {
  const id = String(useParams().id);
  const { data: db, isLoading, error, refetch } = useDatabase(id);
  const [fullscreen, toggleFullscreen] = useBrowserFullscreen();
  const canFullscreen = useFullscreenSupported();
  const back = `/databases/${encodeURIComponent(id)}`;

  return (
    <div className="erd-page">
      <header className="erd-pagebar">
        <Link href={back} className="erd-back" aria-label={`Back to ${db?.name ?? "the database"}`}>
          <ArrowLeft size={16} aria-hidden />
        </Link>
        <div className="erd-pagebar-title">
          {db ? (
            <>
              <span className="erd-pagebar-context">
                {db.instance ? (
                  <Link href={`/instances/${db.instance.id}`} className="link-plain">
                    {db.instance.name}
                  </Link>
                ) : null}
                {db.instance ? " / " : null}
                <Link href={back} className="link-plain">
                  {db.name}
                </Link>
              </span>
              <h1>Diagram</h1>
            </>
          ) : (
            <Skeleton width="12rem" height="1.25rem" />
          )}
        </div>
        <div className="erd-pagebar-actions">
          {db?.instance?.has_credentials ? (
            <ButtonLink
              href={`/data?db=${encodeURIComponent(id)}`}
              size="sm"
              variant="secondary"
              icon={<Table2 size={15} aria-hidden />}
            >
              <span className="hide-sm">Open data browser</span>
            </ButtonLink>
          ) : null}
          {canFullscreen ? (
            <IconButton label={fullscreen ? "Exit full screen" : "Full screen"} onClick={toggleFullscreen}>
              {fullscreen ? <Minimize size={15} aria-hidden /> : <Maximize size={15} aria-hidden />}
            </IconButton>
          ) : null}
          <ThemeToggle size="sm" />
        </div>
      </header>
      <div className="erd-page-body">
        {isLoading ? null : !db ? (
          <div className="erd-state">
            <QueryError error={error} what="database" onRetry={() => void refetch()} backHref="/databases" backLabel="Back to databases" />
          </div>
        ) : !db.instance?.has_credentials ? (
          <div className="erd-state">
            <EmptyState
              icon={<Network size={22} aria-hidden />}
              title="No login for this database server"
              hint="Fleetdock needs a login on the database server to read the tables. Add one on the server's page."
              action={
                db.instance ? (
                  <ButtonLink href={`/instances/${db.instance.id}`} variant="primary">
                    Open {db.instance.name}
                  </ButtonLink>
                ) : null
              }
            />
          </div>
        ) : (
          <ERDiagram database={db} />
        )}
      </div>
    </div>
  );
}
