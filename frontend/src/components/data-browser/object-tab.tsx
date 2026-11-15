"use client";

import { SqlEditor } from "@/components/sql-editor";
import { useCopy, useToast } from "@/components/toast";
import { Badge, Button, EmptyState, Spinner } from "@/components/ui";
import type { Dialect } from "@/lib/data-browser/cells";
import { objectName, type OpenRequest, type Tab } from "@/lib/data-browser/tabs";
import { useDBObjects } from "@/lib/hooks";
import { ClipboardCopy, SquareTerminal, Table2 } from "lucide-react";

import { ObjectIcon } from "./object-tree";

const KIND_LABEL: Record<string, string> = {
  function: "Function",
  procedure: "Procedure",
  trigger: "Trigger",
  sequence: "Sequence",
  event: "Event",
};

const noop = () => {};

/**
 * ObjectTab shows a routine, trigger, sequence or event: what it is, where it
 * lives and its definition. Changing one is done in the SQL console.
 */
export function ObjectTab({
  databaseId,
  tab,
  dialect,
  onOpen,
  onNewQuery,
}: {
  databaseId: string;
  tab: Tab;
  dialect: Dialect;
  onOpen: (req: OpenRequest) => void;
  onNewQuery: (sql?: string) => void;
}) {
  const { data, isLoading, error } = useDBObjects(databaseId);
  const { push } = useToast();
  const copy = useCopy();
  const obj = data?.find((o) => objectName(o) === tab.name);

  if (isLoading) {
    return (
      <div className="dbx-center">
        <Spinner /> Loading…
      </div>
    );
  }
  if (error || !obj) {
    return (
      <div className="dbx-center">
        <EmptyState
          title={error ? "Could not load this object" : "This object no longer exists"}
          hint={error ? (error as Error).message : "It may have been dropped or renamed. Refresh the sidebar to see what is there now."}
        />
      </div>
    );
  }

  // A trigger's table is reported bare; the API addresses PostgreSQL tables as schema.table.
  const tableName = obj.table
    ? dialect === "postgres" && !obj.table.includes(".")
      ? `${obj.schema}.${obj.table}`
      : obj.table
    : null;
  const definition = obj.definition;

  return (
    <div className="dbx-pane">
      <div className="dbx-toolbar">
        <span className="dbx-object-title">
          <ObjectIcon kind={obj.kind} />
          <Badge>{KIND_LABEL[obj.kind] ?? obj.kind}</Badge>
          <span className="font-medium truncate">
            <span className="muted">{obj.schema}.</span>
            {obj.name}
          </span>
        </span>
        {tableName ? (
          <Button
            variant="ghost"
            size="sm"
            icon={<Table2 size={14} aria-hidden />}
            onClick={() => onOpen({ kind: "table", name: tableName, label: obj.table! })}
          >
            on {obj.table}
          </Button>
        ) : null}
        <div className="flex-1" />
        {definition ? (
          <>
            <Button
              size="sm"
              icon={<ClipboardCopy size={14} aria-hidden />}
              onClick={() => copy(definition, "Copied definition")}
            >
              Copy
            </Button>
            <Button size="sm" icon={<SquareTerminal size={14} aria-hidden />} onClick={() => onNewQuery(definition)}>
              Open in SQL console
            </Button>
          </>
        ) : null}
      </div>
      <div className="dbx-structure">
        {definition ? (
          <SqlEditor value={definition} onChange={noop} readOnly minHeight="auto" />
        ) : (
          <EmptyState
            title="No definition available"
            hint="The engine did not return one — the database login may lack the privilege to read it, or this kind of object has none."
          />
        )}
      </div>
    </div>
  );
}
