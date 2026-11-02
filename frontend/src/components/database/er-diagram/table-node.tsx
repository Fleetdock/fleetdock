"use client";

import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import { KeyRound, Link2 } from "lucide-react";
import Link from "next/link";
import { memo } from "react";

import { cx } from "@/components/ui";
import type { DiagramTable } from "@/lib/types";

import { type CardColumns, tableId } from "./graph";

export type TableNodeData = {
  table: DiagramTable;
  cols: CardColumns;
  keys: Set<string>;
  href: string;
  /** Columns of the selected relation(s) on this table, highlighted. */
  activeColumns: Set<string>;
  highlighted: boolean;
  dimmed: boolean;
  /** Ghost cards (a table of a schema that is not shown): add its schema. */
  onShowSchema?: () => void;
};

export type TableNodeType = Node<TableNodeData, "table">;

const compact = new Intl.NumberFormat(undefined, { notation: "compact", maximumFractionDigits: 1 });

/** HEADER_HANDLE anchors relations on a column folded out of the card. */
export const HEADER_HANDLE = "__table";

export const handleId = (column: string, side: "s" | "t") => `${column}:${side}`;

function TableNodeImpl({ data, selected }: NodeProps<TableNodeType>) {
  const { table, cols, keys, href, activeColumns, highlighted, dimmed } = data;
  const id = tableId(table);
  if (table.ghost) {
    return (
      <div className={cx("erd-card erd-card-ghost", selected && "is-selected", dimmed && "is-dimmed")}>
        <div className="erd-card-head">
          <Handle type="target" position={Position.Left} id={handleId(HEADER_HANDLE, "t")} className="erd-handle" />
          <span className="erd-card-title" title={`${id} is in a schema that is not shown`}>
            <span className="erd-card-schema">{table.schema}.</span>
            {table.name}
          </span>
          <Handle type="source" position={Position.Right} id={handleId(HEADER_HANDLE, "s")} className="erd-handle" />
        </div>
        <button type="button" className="erd-ghost-show nodrag" onClick={data.onShowSchema}>
          Show schema {table.schema}
        </button>
      </div>
    );
  }
  return (
    <div
      className={cx(
        "erd-card",
        table.kind === "view" && "erd-card-view",
        selected && "is-selected",
        highlighted && "is-highlighted",
        dimmed && "is-dimmed",
      )}
    >
      <div className="erd-card-head">
        <Handle type="target" position={Position.Left} id={handleId(HEADER_HANDLE, "t")} className="erd-handle" />
        <Link href={href} className="erd-card-title nodrag" title={`Open ${id} in the data browser`}>
          {table.schema ? <span className="erd-card-schema">{table.schema}.</span> : null}
          {table.name}
        </Link>
        <span className="erd-card-meta">
          {table.kind === "view" ? "view" : table.row_estimate !== undefined ? `~${compact.format(table.row_estimate)} rows` : ""}
        </span>
        <Handle type="source" position={Position.Right} id={handleId(HEADER_HANDLE, "s")} className="erd-handle" />
      </div>
      <ul className="erd-cols" aria-label={`Columns of ${id}`}>
        {cols.shown.length === 0 ? <li className="erd-col erd-col-empty">No columns visible</li> : null}
        {cols.shown.map((c) => {
          const fk = keys.has(c.name) && !c.primary_key;
          return (
            <li key={c.name} className={cx("erd-col", activeColumns.has(c.name) && "is-active")}>
              <Handle type="target" position={Position.Left} id={handleId(c.name, "t")} className="erd-handle" />
              <span className={cx("erd-col-icon", c.primary_key && "is-pk")} aria-hidden>
                {c.primary_key ? <KeyRound size={12} /> : fk ? <Link2 size={12} /> : null}
              </span>
              <span className="erd-col-name">
                {c.name}
                {c.primary_key ? <span className="sr-only"> (primary key)</span> : null}
                {fk ? <span className="sr-only"> (foreign key)</span> : null}
              </span>
              <span className="erd-col-type">
                {c.type}
                {c.nullable ? <span className="erd-col-null" title="Accepts NULL">?</span> : null}
              </span>
              <Handle type="source" position={Position.Right} id={handleId(c.name, "s")} className="erd-handle" />
            </li>
          );
        })}
      </ul>
      {cols.hidden > 0 ? (
        <div className="erd-card-more">
          +{cols.hidden} more column{cols.hidden === 1 ? "" : "s"}
        </div>
      ) : null}
    </div>
  );
}

export const TableNode = memo(TableNodeImpl);
