"use client";

import { useRouter } from "next/navigation";
import { useDeferredValue, useEffect, useId, useMemo, useState, type ReactNode } from "react";

import { Database, FileText, HardDrive, Server, type LucideIcon } from "lucide-react";

import { useVisibleSections } from "@/components/sidebar";
import { Modal } from "@/components/ui";
import { useCanAny, useDatabases, useInstances, useServers } from "@/lib/hooks";

const OPEN_EVENT = "fleetdock:open-palette";

type Item = { id: string; label: string; hint?: string; href: string; Icon: LucideIcon; group: string };

/** Section shortcuts: "g" then the letter jumps there. */
export const GO_KEYS: Record<string, string> = {
  o: "/dashboard",
  s: "/servers",
  d: "/databases",
  t: "/data",
  b: "/backups",
  a: "/activity",
  u: "/access/users",
  n: "/settings/notifications",
  p: "/settings/profile",
};

function isTyping(t: EventTarget | null) {
  const el = t as HTMLElement | null;
  return !!el && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName));
}

/**
 * CommandPalette (Cmd/Ctrl+K) jumps to any page, server, database server or
 * database. It also owns the global shortcuts: "?" for help and "g" + letter
 * to go to a section.
 */
export function CommandPalette() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [help, setHelp] = useState(false);

  useEffect(() => {
    let pendingG = 0;
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((o) => !o);
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey || isTyping(e.target) || document.querySelector('[role="dialog"]')) return;
      if (e.key === "?") {
        e.preventDefault();
        setHelp(true);
      } else if (e.key === "/") {
        e.preventDefault();
        setOpen(true);
      } else if (e.key === "g") {
        pendingG = Date.now();
      } else if (pendingG && Date.now() - pendingG < 1200 && GO_KEYS[e.key]) {
        e.preventDefault();
        pendingG = 0;
        router.push(GO_KEYS[e.key]);
      } else {
        pendingG = 0;
      }
    };
    const onOpen = () => setOpen(true);
    document.addEventListener("keydown", onKey);
    window.addEventListener(OPEN_EVENT, onOpen);
    return () => {
      document.removeEventListener("keydown", onKey);
      window.removeEventListener(OPEN_EVENT, onOpen);
    };
  }, [router]);

  return (
    <>
      {open ? <Palette onClose={() => setOpen(false)} /> : null}
      <Modal open={help} onClose={() => setHelp(false)} title="Keyboard shortcuts">
        <ShortcutHelp />
      </Modal>
    </>
  );
}

function Palette({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const listId = useId();
  const [query, setQuery] = useState("");
  // Local matches use what is typed right now, so Enter always opens what is
  // shown; only the server-side database search is deferred.
  const q = query.trim().toLowerCase();
  const remoteQ = useDeferredValue(q);
  const [active, setActive] = useState(0);
  const sections = useVisibleSections();
  const canAny = useCanAny();

  const { data: servers } = useServers(undefined, undefined, canAny("server:read"));
  const { data: instances } = useInstances(undefined, undefined, undefined, canAny("instance:read"));
  // The databases endpoint searches server-side; the others are filtered here.
  const { data: dbResults } = useDatabases({ search: remoteQ, page: 1, enabled: remoteQ !== "" && canAny("database:read") });
  const databases = q === remoteQ ? dbResults : undefined;

  const items = useMemo(() => {
    const out: Item[] = [];
    for (const s of sections) {
      for (const p of s.pages) {
        const label = s.pages.length > 1 ? `${s.label} › ${p.label}` : p.label;
        if (!q || `${label} ${p.keywords ?? ""}`.toLowerCase().includes(q)) {
          out.push({ id: `page:${p.href}`, label, href: p.href, Icon: FileText, group: "Pages" });
        }
      }
    }
    if (q) {
      for (const s of servers?.items ?? []) {
        if (`${s.name} ${s.hostname}`.toLowerCase().includes(q)) {
          out.push({ id: `server:${s.id}`, label: s.name, hint: s.hostname, href: `/servers/${s.id}`, Icon: Server, group: "Servers" });
        }
      }
      for (const i of instances?.items ?? []) {
        if (i.name.toLowerCase().includes(q)) {
          out.push({ id: `instance:${i.id}`, label: i.name, hint: `${i.engine} ${i.engine_version}`, href: `/instances/${i.id}`, Icon: HardDrive, group: "Database servers" });
        }
      }
      // Previous results linger while a new search loads; keep only matches.
      for (const d of (databases?.items ?? []).filter((d) => d.name.toLowerCase().includes(q))) {
        out.push({ id: `db:${d.id}`, label: d.name, hint: d.instance?.name, href: `/databases/${d.id}`, Icon: Database, group: "Databases" });
      }
    }
    return out.slice(0, 50);
  }, [sections, q, servers, instances, databases]);

  const current = Math.min(active, Math.max(0, items.length - 1));

  function go(item: Item | undefined) {
    if (!item) return;
    onClose();
    router.push(item.href);
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((current + 1) % Math.max(1, items.length));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((current - 1 + items.length) % Math.max(1, items.length));
    } else if (e.key === "Enter") {
      e.preventDefault();
      go(items[current]);
    }
  }

  useEffect(() => {
    document.getElementById(`${listId}-${current}`)?.scrollIntoView({ block: "nearest" });
  }, [current, listId]);

  let lastGroup = "";
  return (
    <Modal open onClose={onClose} title="Go to…">
      <input
        className="input"
        autoFocus
        role="combobox"
        aria-expanded
        aria-controls={listId}
        aria-activedescendant={items.length ? `${listId}-${current}` : undefined}
        aria-label="Search pages, servers and databases"
        placeholder="Search pages, servers and databases…"
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          setActive(0);
        }}
        onKeyDown={onKeyDown}
      />
      <ul id={listId} role="listbox" className="palette-list" aria-label="Results">
        {items.length === 0 ? (
          <li className="muted text-sm" style={{ padding: ".8rem .5rem" }}>
            Nothing found
          </li>
        ) : (
          items.map((it, i) => {
            const header: ReactNode =
              it.group !== lastGroup ? (
                <li role="presentation" className="palette-group">
                  {it.group}
                </li>
              ) : null;
            lastGroup = it.group;
            return [
              header,
              <li
                key={it.id}
                id={`${listId}-${i}`}
                role="option"
                aria-selected={i === current}
                className={`palette-item${i === current ? " active" : ""}`}
                onMouseMove={() => setActive(i)}
                onClick={() => go(it)}
              >
                <it.Icon size={15} aria-hidden className="muted" />
                <span className="truncate" style={{ flex: 1 }}>
                  {it.label}
                </span>
                {it.hint ? <span className="muted text-sm truncate">{it.hint}</span> : null}
              </li>,
            ];
          })
        )}
      </ul>
      <p className="muted text-sm" style={{ margin: ".6rem 0 0" }}>
        <kbd>↑</kbd> <kbd>↓</kbd> to move · <kbd>Enter</kbd> to open · <kbd>Esc</kbd> to close
      </p>
    </Modal>
  );
}

function ShortcutHelp() {
  const rows: [string[][], string][] = [
    [[["Ctrl", "K"], ["⌘", "K"], ["/"]], "Search and go anywhere"],
    [[["?"]], "Show this help"],
    [[["g", "o"]], "Overview"],
    [[["g", "s"]], "Servers"],
    [[["g", "d"]], "Databases"],
    [[["g", "t"]], "Data browser"],
    [[["g", "b"]], "Backups"],
    [[["g", "a"]], "Activity"],
    [[["g", "u"]], "Users"],
    [[["g", "n"]], "Notifications"],
    [[["g", "p"]], "Your profile"],
    [[["Esc"]], "Close a dialog or menu"],
  ];
  return (
    <table className="table">
      <tbody>
        {rows.map(([combos, label]) => (
          <tr key={label}>
            <td style={{ width: "50%" }}>
              {combos.map((keys, i) => (
                <span key={keys.join("+")}>
                  {i > 0 ? <span className="muted"> or </span> : null}
                  {keys.map((k) => (
                    <kbd key={k} style={{ marginRight: ".2rem" }}>
                      {k}
                    </kbd>
                  ))}
                </span>
              ))}
            </td>
            <td>{label}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** openCommandPalette opens the palette from anywhere (e.g. the topbar button). */
export function openCommandPalette() {
  window.dispatchEvent(new Event(OPEN_EVENT));
}
