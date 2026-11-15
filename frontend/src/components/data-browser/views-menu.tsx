"use client";

import { Bookmark, ChevronDown, Link2, Pencil, Trash2, Users } from "lucide-react";
import { useState, type FormEvent } from "react";

import { useConfirm } from "@/components/confirm";
import { useCopy, useToast } from "@/components/toast";
import { Button, Checkbox, ErrorText, Field, IconButton, Input, Modal, cx } from "@/components/ui";
import type { TableView } from "@/lib/data-browser/tabs";
import { isModified, pickSavedState, savedViewLink } from "@/lib/data-browser/view-state";
import { friendlyError } from "@/lib/errors";
import { useMe, useTableViewMutations, useTableViews } from "@/lib/hooks";
import type { SavedTableView } from "@/lib/types";

import { Popover } from "./popover";

/**
 * ViewsMenu saves the tab's filters, search, sort and columns as a named view
 * of the table, and applies saved ones: yours, and those others shared.
 */
export function ViewsMenu({
  databaseId,
  table,
  view,
  onApply,
  onClearSaved,
}: {
  databaseId: string;
  table: string;
  view: TableView;
  onApply: (v: SavedTableView) => void;
  /** The active view was deleted. */
  onClearSaved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<{ view?: SavedTableView } | null>(null);
  const { data: views } = useTableViews(databaseId, table);
  const { update, remove } = useTableViewMutations(databaseId, table);
  const { data: me } = useMe();
  const { push } = useToast();
  const copy = useCopy();
  const confirm = useConfirm();

  const active = views?.find((v) => v.id === view.savedView);
  const changed = active ? isModified(view, active.state) : false;
  const mine = (views ?? []).filter((v) => v.created_by === me?.id);
  const shared = (views ?? []).filter((v) => v.created_by !== me?.id);

  const copyLink = (v: SavedTableView) =>
    copy(
      savedViewLink(window.location.origin, databaseId, table, v.id),
      "Link copied. People who can open this database see the view if it is shared.",
    );

  const del = async (v: SavedTableView) => {
    const ok = await confirm({
      title: `Delete the view “${v.name}”?`,
      message: v.shared ? "It is shared: it disappears for everyone." : "Only the saved view goes; the table is not touched.",
      confirmLabel: "Delete view",
      danger: true,
    });
    if (!ok) return;
    try {
      await remove.mutateAsync(v.id);
      if (v.id === view.savedView) onClearSaved();
      push("success", `Deleted the view “${v.name}”`);
    } catch (err) {
      push("error", friendlyError(err, "The view could not be deleted."));
    }
  };

  const saveChanges = async () => {
    if (!active) return;
    try {
      await update.mutateAsync({ id: active.id, state: pickSavedState(view) as unknown as Record<string, unknown> });
      push("success", `Updated the view “${active.name}”`);
    } catch (err) {
      push("error", friendlyError(err, "The view could not be updated."));
    }
  };

  const row = (v: SavedTableView) => (
    <li key={v.id} className={cx("dbx-view-row", v.id === view.savedView && "is-active")}>
      <button
        type="button"
        className="dbx-view-apply"
        onClick={() => {
          onApply(v);
          setOpen(false);
        }}
        title={v.shared && v.author_email ? `Shared by ${v.author_email}` : undefined}
      >
        <span className="truncate">{v.name}</span>
        {v.shared ? <Users size={12} aria-label="shared" className="muted flex-none" /> : null}
        {v.created_by !== me?.id && v.author_email ? (
          <span className="muted text-xs truncate">{v.author_email}</span>
        ) : null}
      </button>
      <span className="dbx-view-actions">
        <IconButton label={`Copy a link to ${v.name}`} onClick={() => copyLink(v)}>
          <Link2 size={13} aria-hidden />
        </IconButton>
        {v.created_by === me?.id ? (
          <IconButton label={`Rename or share ${v.name}`} onClick={() => setEditing({ view: v })}>
            <Pencil size={13} aria-hidden />
          </IconButton>
        ) : null}
        <IconButton label={`Delete ${v.name}`} onClick={() => void del(v)}>
          <Trash2 size={13} aria-hidden />
        </IconButton>
      </span>
    </li>
  );

  return (
    <>
      <Popover
        open={open}
        onClose={() => setOpen(false)}
        label="Saved views"
        align="right"
        className="dbx-views-pop"
        trigger={
          <Button
            size="sm"
            icon={<Bookmark size={14} aria-hidden />}
            active={Boolean(active)}
            aria-expanded={open}
            onClick={() => setOpen((v) => !v)}
            title={active ? `View: ${active.name}${changed ? " (changed)" : ""}` : "Saved views of this table"}
          >
            <span className="dbx-views-label">{active ? active.name : "Views"}</span>
            {changed ? <span className="dbx-views-dot" aria-label="changed since saved" /> : null}
            <ChevronDown size={13} aria-hidden />
          </Button>
        }
      >
        {(views ?? []).length === 0 ? (
          <p className="muted text-sm m-0 p-2">
            Save the filters, search, sort and columns you set up here as a view, to come back to them in one click or
            share them.
          </p>
        ) : null}
        {mine.length ? (
          <>
            <div className="dbx-view-group">Your views</div>
            <ul className="dbx-columns-list">{mine.map(row)}</ul>
          </>
        ) : null}
        {shared.length ? (
          <>
            <div className="dbx-view-group">Shared with you</div>
            <ul className="dbx-columns-list">{shared.map(row)}</ul>
          </>
        ) : null}
        <div className="dbx-columns-foot gap-2">
          {active && changed && active.created_by === me?.id ? (
            <Button size="sm" loading={update.isPending} onClick={() => void saveChanges()}>
              Update “{active.name}”
            </Button>
          ) : null}
          <Button
            size="sm"
            variant="primary"
            onClick={() => {
              setOpen(false);
              setEditing({});
            }}
          >
            Save current view…
          </Button>
        </div>
      </Popover>
      {editing ? (
        <SaveViewModal
          databaseId={databaseId}
          table={table}
          view={view}
          existing={editing.view}
          onClose={() => setEditing(null)}
          onSaved={(v) => {
            setEditing(null);
            if (!editing.view) onApply(v);
          }}
        />
      ) : null}
    </>
  );
}

function SaveViewModal({
  databaseId,
  table,
  view,
  existing,
  onClose,
  onSaved,
}: {
  databaseId: string;
  table: string;
  view: TableView;
  /** Renaming or sharing an existing view instead of saving a new one. */
  existing?: SavedTableView;
  onClose: () => void;
  onSaved: (v: SavedTableView) => void;
}) {
  const [name, setName] = useState(existing?.name ?? "");
  const [shared, setShared] = useState(existing?.shared ?? false);
  const [error, setError] = useState<string | null>(null);
  const { create, update } = useTableViewMutations(databaseId, table);
  const { push } = useToast();
  const busy = create.isPending || update.isPending;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const v = existing
        ? await update.mutateAsync({ id: existing.id, name, shared })
        : await create.mutateAsync({ name, shared, state: pickSavedState(view) as unknown as Record<string, unknown> });
      push("success", existing ? `Saved “${v.name}”` : `Saved the view “${v.name}”`);
      onSaved(v);
    } catch (err) {
      setError(friendlyError(err, "The view could not be saved."));
    }
  }

  const summary = [
    view.filters.length ? `${view.filters.length} filter${view.filters.length === 1 ? "" : "s"}` : null,
    view.search ? `search “${view.search}”` : null,
    view.sort.length ? `sorted by ${view.sort.map((s) => s.column).join(", ")}` : null,
    view.hidden?.length ? `${view.hidden.length} hidden column${view.hidden.length === 1 ? "" : "s"}` : null,
    view.frozen ? `${view.frozen} frozen` : null,
  ].filter(Boolean);

  return (
    <Modal open onClose={onClose} title={existing ? "Rename or share view" : "Save view"}>
      <form onSubmit={(e) => void onSubmit(e)} className="stack">
        <Field label="Name">
          <Input value={name} autoFocus maxLength={100} onChange={(e) => setName(e.target.value)} placeholder="Paid orders this month" />
        </Field>
        {!existing ? (
          <p className="muted text-sm m-0">
            {summary.length ? `Keeps ${summary.join(", ")}.` : "Keeps the column layout; there are no filters or sort yet."}
          </p>
        ) : null}
        <Checkbox
          label="Share with everyone who can open this database"
          hint="They see it in their list and can apply it; only you can change it."
          checked={shared}
          onChange={(e) => setShared(e.target.checked)}
        />
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={busy} disabled={!name.trim()}>
            {existing ? "Save" : "Save view"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
