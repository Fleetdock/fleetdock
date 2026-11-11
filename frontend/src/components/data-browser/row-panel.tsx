"use client";

import { ChevronDown, ChevronUp, ClipboardCopy, KeyRound, X } from "lucide-react";
import { useState } from "react";

import { useCopy, useToast } from "@/components/toast";
import { Button, IconButton, Textarea } from "@/components/ui";
import { cellKind, isTruncated, parseJsonDoc, prettyValue } from "@/lib/data-browser/cells";
import type { BrowseColumn } from "@/lib/types";

import { cellState, type GridRow } from "./data-grid";
import { JsonViewer } from "./json-viewer";

/**
 * RowPanel shows the cursor's whole row as a form: every column with its
 * type and full value, editable field by field. Edits go to the tab's pending
 * changes like grid edits; nothing is written until the tab is saved.
 */
export function RowPanel({
  row,
  label,
  columns,
  keyCols,
  editable,
  onApply,
  onPrev,
  onNext,
  onClose,
}: {
  row?: GridRow;
  label: string;
  columns: BrowseColumn[];
  keyCols: string[];
  /** The row can be edited (write access, a row key, not deleted). */
  editable: boolean;
  onApply: (ci: number, value: string | null) => void;
  onPrev?: () => void;
  onNext?: () => void;
  onClose: () => void;
}) {
  const [editing, setEditing] = useState<{ ci: number; text: string } | null>(null);
  const rowKey = row ? (row.kind === "new" ? row.id : `d${row.index}`) : "";
  const [prevKey, setPrevKey] = useState(rowKey);
  if (prevKey !== rowKey) {
    setPrevKey(rowKey);
    setEditing(null);
  }
  const { push } = useToast();
  const copy = useCopy();

  return (
    <aside className="dbx-valuepanel dbx-rowpanel" aria-label="Row panel">
      <div className="dbx-valuepanel-head">
        <span className="truncate font-medium">{row ? label : "Row"}</span>
        <div className="flex-1" />
        <IconButton label="Previous row" onClick={onPrev} disabled={!onPrev}>
          <ChevronUp size={14} aria-hidden />
        </IconButton>
        <IconButton label="Next row" onClick={onNext} disabled={!onNext}>
          <ChevronDown size={14} aria-hidden />
        </IconButton>
        <IconButton label="Close row panel" onClick={onClose}>
          <X size={14} aria-hidden />
        </IconButton>
      </div>
      {!row ? (
        <p className="muted text-sm p-3">Select a row to see all of its columns here. Shift+Enter opens this panel from the grid.</p>
      ) : (
        <dl className="dbx-rowpanel-list">
          {columns.map((c, ci) => {
            const st = cellState(row, c, ci);
            const kind = cellKind(c.type);
            const locked = isTruncated(st.original) && !st.changed;
            const canEdit = editable && !c.generated && !locked;
            const ed = editing?.ci === ci ? editing : null;
            const jsonDoc = parseJsonDoc(st.value);
            return (
              <div key={c.name} className={`dbx-rowpanel-field${st.changed ? " is-changed" : ""}`}>
                <dt>
                  {keyCols.includes(c.name) ? <KeyRound size={11} className="dbx-key" aria-label="key" /> : null}
                  <span className="truncate">{c.name}</span>
                  <span className="muted text-xs truncate">{c.type}</span>
                </dt>
                <dd>
                  {ed ? (
                    <div className="stack-sm">
                      <Textarea
                        className="dbx-rowpanel-edit"
                        value={ed.text}
                        autoFocus
                        spellCheck={false}
                        rows={Math.min(10, Math.max(2, ed.text.split("\n").length))}
                        onChange={(e) => setEditing({ ci, text: e.target.value })}
                        onKeyDown={(e) => {
                          if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
                            onApply(ci, ed.text);
                            setEditing(null);
                          } else if (e.key === "Escape") {
                            e.stopPropagation();
                            setEditing(null);
                          }
                        }}
                        aria-label={`Edit ${c.name}`}
                      />
                      <div className="flex flex-wrap gap-1.5">
                        <Button
                          size="sm"
                          variant="primary"
                          onClick={() => {
                            onApply(ci, ed.text);
                            setEditing(null);
                          }}
                        >
                          Apply
                        </Button>
                        <Button size="sm" onClick={() => setEditing(null)}>
                          Cancel
                        </Button>
                        {kind === "json" || parseJsonDoc(ed.text) ? (
                          <Button size="sm" variant="ghost" onClick={() => setEditing({ ci, text: prettyValue(ed.text, "json") })}>
                            Format JSON
                          </Button>
                        ) : null}
                      </div>
                    </div>
                  ) : (
                    <div className="dbx-rowpanel-value">
                      {st.isDefault ? (
                        <span className="dbx-pill">DEFAULT</span>
                      ) : st.value === null ? (
                        <span className="dbx-pill">NULL</span>
                      ) : st.value === "" ? (
                        <span className="dbx-empty">empty</span>
                      ) : jsonDoc ? (
                        <JsonViewer key={`${rowKey}:${c.name}`} doc={jsonDoc} compact />
                      ) : (
                        <pre className="dbx-rowpanel-pre">{prettyValue(st.value, kind)}</pre>
                      )}
                      <span className="dbx-rowpanel-actions">
                        {st.value ? (
                          <IconButton
                            label={`Copy ${c.name}`}
                            onClick={() => copy(st.value ?? "", "Copied value")}
                          >
                            <ClipboardCopy size={13} aria-hidden />
                          </IconButton>
                        ) : null}
                        {canEdit ? (
                          <Button size="sm" variant="ghost" onClick={() => setEditing({ ci, text: st.value ?? "" })}>
                            Edit
                          </Button>
                        ) : null}
                        {canEdit && c.nullable && st.value !== null ? (
                          <Button size="sm" variant="ghost" onClick={() => onApply(ci, null)}>
                            NULL
                          </Button>
                        ) : null}
                      </span>
                    </div>
                  )}
                  {locked ? <p className="muted text-xs m-0">Shown cut short at 4 KB; edit it in the SQL console.</p> : null}
                </dd>
              </div>
            );
          })}
        </dl>
      )}
    </aside>
  );
}
