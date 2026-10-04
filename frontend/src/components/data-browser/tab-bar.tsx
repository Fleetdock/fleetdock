"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";

import type { Tab } from "@/lib/data-browser/tabs";
import { Copy, Eye, Plus, SquareTerminal, Table2, X } from "lucide-react";

import { useContextMenu } from "./context-menu";

export type CloseKind = "close" | "closeOthers" | "closeRight" | "closeAll";

/**
 * TabBar shows the open tables, views and queries. Click switches, middle
 * click or × closes, drag reorders, right click offers bulk close. A dot marks
 * a tab with unsaved edits; an italic label marks a preview tab.
 */
export function TabBar({
  tabs,
  active,
  dirty,
  onActivate,
  onPin,
  onClose,
  onMove,
  onNewQuery,
  leading,
}: {
  tabs: Tab[];
  active: string | null;
  dirty: Record<string, number>;
  onActivate: (id: string) => void;
  onPin: (id: string) => void;
  onClose: (kind: CloseKind, id: string) => void;
  onMove: (id: string, to: number) => void;
  onNewQuery: () => void;
  /** Controls shown before the tabs (sidebar toggle). */
  leading?: ReactNode;
}) {
  const strip = useRef<HTMLDivElement>(null);
  const [drag, setDrag] = useState<{ id: string; over: number | null } | null>(null);
  const ctx = useContextMenu();

  // Keep the active tab in view when it changes (opened from the sidebar, cycled by key).
  useEffect(() => {
    if (!active) return;
    strip.current
      ?.querySelector<HTMLElement>(`[data-tab="${CSS.escape(active)}"]`)
      ?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [active]);

  function onKeyDown(e: React.KeyboardEvent, i: number) {
    let next = -1;
    if (e.key === "ArrowRight") next = (i + 1) % tabs.length;
    else if (e.key === "ArrowLeft") next = (i - 1 + tabs.length) % tabs.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = tabs.length - 1;
    else if (e.key === "Delete") {
      e.preventDefault();
      onClose("close", tabs[i].id);
      return;
    }
    if (next >= 0) {
      e.preventDefault();
      onActivate(tabs[next].id);
      strip.current?.querySelector<HTMLElement>(`[data-tab="${CSS.escape(tabs[next].id)}"]`)?.focus();
    }
  }

  return (
    <div className="dbx-tabbar">
      {leading}
      <div
        className="dbx-tabs"
        ref={strip}
        role="tablist"
        aria-label="Open tables and queries"
        onDoubleClick={(e) => {
          // Double-click on the empty strip opens a new query, like a browser.
          if (e.target === e.currentTarget) onNewQuery();
        }}
      >
        {tabs.map((t, i) => {
          const isActive = t.id === active;
          const changes = dirty[t.id] ?? 0;
          const icon =
            t.kind === "query" ? (
              <SquareTerminal size={13} className="dbx-tab-icon query" />
            ) : t.kind === "view" ? (
              <Eye size={13} className="dbx-tab-icon view" />
            ) : (
              <Table2 size={13} className="dbx-tab-icon" />
            );
          const dropSide = drag && drag.over === i && drag.id !== t.id ? (tabs.findIndex((x) => x.id === drag.id) < i ? " drop-after" : " drop-before") : "";
          return (
            <div
              key={t.id}
              data-tab={t.id}
              role="tab"
              tabIndex={isActive ? 0 : -1}
              aria-selected={isActive}
              aria-controls={`pane-${t.id}`}
              className={`dbx-tab${isActive ? " active" : ""}${t.preview ? " preview" : ""}${dropSide}`}
              title={`${t.kind === "query" ? t.label : t.name}${changes ? ` — ${changes} unsaved change${changes === 1 ? "" : "s"}` : ""}${t.preview ? " (preview — double-click to keep open)" : ""}`}
              draggable
              onDragStart={(e) => {
                e.dataTransfer.effectAllowed = "move";
                e.dataTransfer.setData("text/plain", t.id);
                setDrag({ id: t.id, over: null });
              }}
              onDragOver={(e) => {
                if (!drag) return;
                e.preventDefault();
                if (drag.over !== i) setDrag({ ...drag, over: i });
              }}
              onDrop={(e) => {
                e.preventDefault();
                if (drag && drag.id !== t.id) onMove(drag.id, i);
                setDrag(null);
              }}
              onDragEnd={() => setDrag(null)}
              onMouseDown={(e) => {
                if (e.button === 1) e.preventDefault(); // no autoscroll on middle click
              }}
              onClick={() => onActivate(t.id)}
              onDoubleClick={() => onPin(t.id)}
              onAuxClick={(e) => {
                if (e.button === 1) onClose("close", t.id);
              }}
              onKeyDown={(e) => onKeyDown(e, i)}
              onContextMenu={(e) =>
                ctx.open(e, [
                  { label: "Close", icon: <X size={14} />, onSelect: () => onClose("close", t.id), hint: "Alt W" },
                  { label: "Close others", onSelect: () => onClose("closeOthers", t.id), disabled: tabs.length < 2 },
                  { label: "Close to the right", onSelect: () => onClose("closeRight", t.id), disabled: i === tabs.length - 1 },
                  { label: "Close all", onSelect: () => onClose("closeAll", t.id) },
                  "separator",
                  { label: "Keep open", onSelect: () => onPin(t.id), hidden: !t.preview },
                  {
                    label: "Copy name",
                    icon: <Copy size={14} />,
                    hidden: t.kind === "query",
                    onSelect: () => void navigator.clipboard?.writeText(t.name),
                  },
                ])
              }
            >
              {icon}
              <span className="dbx-tab-label">{t.label}</span>
              {changes ? <span className="dbx-tab-dirty" aria-label="Unsaved changes" /> : null}
              <button
                type="button"
                className="dbx-tab-close"
                tabIndex={-1}
                aria-label={`Close ${t.label}`}
                onClick={(e) => {
                  e.stopPropagation();
                  onClose("close", t.id);
                }}
              >
                <X size={12} />
              </button>
            </div>
          );
        })}
      </div>
      <button className="btn btn-ghost btn-icon dbx-tab-new" onClick={onNewQuery} title="New SQL query" aria-label="New SQL query">
        <Plus size={15} />
      </button>
      {ctx.element}
    </div>
  );
}
