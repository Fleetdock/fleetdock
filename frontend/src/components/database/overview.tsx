"use client";

import { Archive, ArrowRight, CalendarClock, Globe, Lock } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { connectionURL } from "@/components/connectivity/connectivity-section";
import { BackupDatabaseModal } from "@/components/database/backup-database-modal";
import { IndexHealthCard } from "@/components/database/index-health";
import { Badge, Button, KeyValueList, Skeleton, StatusBadge, Time } from "@/components/ui";
import { engineLabel } from "@/lib/engines";
import { formatBytes } from "@/lib/format";
import { useBackups, useConnectivity, useSchedules } from "@/lib/hooks";
import type { Database } from "@/lib/types";
import { engineSupports } from "@/lib/engines";

/**
 * DatabaseOverview is the database page's landing tab: what the database is,
 * how to reach it, whether it is backed up, and index findings. Each card
 * links to the tab with the full detail.
 */
export function DatabaseOverview({
  database,
  hasCreds,
  canBackup,
  backups = true,
  tabHref,
}: {
  database: Database;
  hasCreds: boolean;
  canBackup: boolean;
  /** Whether the engine supports backups at all. */
  backups?: boolean;
  tabHref: (tab: string) => string;
}) {
  const instance = database.instance;
  const browserHref = `/data?db=${encodeURIComponent(database.id)}`;
  return (
    <div className="db-overview">
      <div className="card panel aside-card">
        <h2 className="panel-title">Details</h2>
        <KeyValueList
          items={[
            {
              label: "Server",
              value: instance ? (
                <Link href={`/instances/${instance.id}`} className="link">
                  {instance.name}
                </Link>
              ) : (
                database.instance_id.slice(0, 8)
              ),
            },
            { label: "Engine", value: instance ? engineLabel(instance.engine) : "—" },
            { label: "Charset", value: database.charset || "—", mono: true },
            { label: "Collation", value: database.collation || "—", mono: true },
            { label: "Created", value: <Time value={database.created_at} /> },
            { label: "Updated", value: <Time value={database.updated_at} /> },
            { label: "Tags", value: database.tags.join(", "), hidden: database.tags.length === 0 },
          ]}
        />
      </div>
      <ConnectCard database={database} manageHref={tabHref("connection")} />
      {backups ? <BackupCard database={database} canBackup={canBackup} manageHref={tabHref("backups")} /> : null}
      {hasCreds && engineSupports(database.instance?.engine, "insights") ? (
        <IndexHealthCard databaseId={database.id} browserHref={browserHref} />
      ) : null}
    </div>
  );
}

function CardLink({ href, children }: { href: string; children: string }) {
  return (
    <Link href={href} className="db-overview-more link" scroll={false}>
      {children} <ArrowRight size={13} aria-hidden />
    </Link>
  );
}

function ConnectCard({ database, manageHref }: { database: Database; manageHref: string }) {
  const { data, isLoading } = useConnectivity(database.id);
  const pub = data?.public;
  const publicOn = Boolean(pub && pub.status !== "disabled");
  return (
    <div className="card panel aside-card">
      <h2 className="panel-title">Connect</h2>
      {isLoading ? (
        <Skeleton height="4.5rem" />
      ) : !data ? (
        <p className="text-sm muted m-0">Connection details are not available right now.</p>
      ) : (
        <KeyValueList
          items={[
            {
              label: "Private",
              value: `${data.private.host}:${data.private.port}`,
              mono: true,
              copy: connectionURL(data.private.protocol, data.private.host, data.private.port, database.name),
            },
            {
              label: "Public",
              value: publicOn ? (
                <span className="inline-flex items-center gap-1.5">
                  <Globe size={13} aria-hidden />
                  <span className="mono">
                    {pub!.host}:{pub!.port}
                  </span>
                  <StatusBadge status={pub!.status} />
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5 muted">
                  <Lock size={13} aria-hidden /> Off
                </span>
              ),
            },
          ]}
        />
      )}
      <CardLink href={manageHref}>Public access and app credentials</CardLink>
    </div>
  );
}

function BackupCard({ database, canBackup, manageHref }: { database: Database; canBackup: boolean; manageHref: string }) {
  const { data: backups, isLoading } = useBackups(database.id, 1);
  const { data: schedules } = useSchedules(database.id);
  const [open, setOpen] = useState(false);
  const last = backups?.items[0];
  const active = (schedules?.items ?? []).filter((s) => s.enabled);
  const next = active
    .map((s) => s.next_run_at)
    .filter((t): t is string => Boolean(t))
    .sort()[0];
  return (
    <div className="card panel aside-card">
      <h2 className="panel-title">Backups</h2>
      {isLoading ? (
        <Skeleton height="4.5rem" />
      ) : (
        <KeyValueList
          items={[
            {
              label: "Last backup",
              value: last ? (
                <span className="inline-flex items-center gap-1.5 flex-wrap">
                  <StatusBadge status={last.status} />
                  <Time value={last.completed_at ?? last.created_at} />
                  {last.size_bytes ? <span className="muted">{formatBytes(last.size_bytes)}</span> : null}
                </span>
              ) : (
                <span className="muted">Never</span>
              ),
            },
            {
              label: "Schedule",
              value:
                active.length > 0 ? (
                  <span className="inline-flex items-center gap-1.5">
                    <CalendarClock size={13} aria-hidden />
                    {next ? (
                      <>
                        next <Time value={next} />
                      </>
                    ) : (
                      "on"
                    )}
                  </span>
                ) : (
                  <Badge tone="warning">No schedule</Badge>
                ),
            },
          ]}
        />
      )}
      <div className="db-overview-actions">
        {canBackup ? (
          <Button size="sm" icon={<Archive size={14} aria-hidden />} onClick={() => setOpen(true)}>
            Back up now
          </Button>
        ) : null}
        <CardLink href={manageHref}>All backups</CardLink>
      </div>
      <BackupDatabaseModal database={open ? database : null} onClose={() => setOpen(false)} />
    </div>
  );
}
