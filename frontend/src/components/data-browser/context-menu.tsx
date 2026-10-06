"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

export type ContextItem =
  | {
      label: ReactNode;
      icon?: ReactNode;
      onSelect: () => void;
      danger?: boolean;
      disabled?: boolean;
      hidden?: boolean;
      /** Shortcut hint shown on the right. */
      hint?: string;
    }
  | "separator";

type Open = { x: number; y: number; items: ContextItem[] };

/**
 * useContextMenu opens a right-click menu at the pointer. Render `element`
 * once; call `open(event, items)` from onContextMenu. Arrow keys move, Enter
 * selects, Esc, scrolling or a click elsewhere close it.
 */
export function useContextMenu() {
  const [menu, setMenu] = useState<Open | null>(null);
  const close = useCallback(() => setMenu(null), []);
  const open = useCallback((e: { clientX: number; clientY: number; preventDefault: () => void }, items: ContextItem[]) => {
    e.preventDefault();
    setMenu({ x: e.clientX, y: e.clientY, items });
  }, []);
  /** openAt anchors the menu below an element (keyboard / button access). */
  const openAt = useCallback((el: HTMLElement, items: ContextItem[]) => {
    const r = el.getBoundingClientRect();
    setMenu({ x: r.left, y: r.bottom + 2, items });
  }, []);
  const element = menu ? <ContextMenu {...menu} onClose={close} /> : null;
  return { open, openAt, close, element };
}

function ContextMenu({ x, y, items, onClose }: Open & { onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });
  const restore = useRef<HTMLElement | null>(null);

  // Keep the menu inside the viewport.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    setPos({
      left: Math.max(4, Math.min(x, window.innerWidth - r.width - 4)),
      top: Math.max(4, y + r.height > window.innerHeight - 4 ? y - r.height : y),
    });
  }, [x, y]);

  useEffect(() => {
    restore.current = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLElement>('[role="menuitem"]:not([disabled])')?.focus();
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onScroll = (e: Event) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", onDown);
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onClose);
    window.addEventListener("blur", onClose);
    return () => {
      document.removeEventListener("mousedown", onDown);
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("blur", onClose);
      restore.current?.focus?.({ preventScroll: true });
    };
  }, [onClose]);

  function onKeyDown(e: React.KeyboardEvent) {
    const nodes = Array.from(ref.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])') ?? []);
    const i = nodes.indexOf(document.activeElement as HTMLElement);
    e.stopPropagation();
    if (e.key === "Escape" || e.key === "Tab") {
      e.preventDefault();
      onClose();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      nodes[(i + 1) % nodes.length]?.focus();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      nodes[(i - 1 + nodes.length) % nodes.length]?.focus();
    }
  }

  const visible = items.filter((it) => it === "separator" || !it.hidden);
  // Drop leading, trailing and doubled separators left by hidden items.
  const cleaned = visible.filter(
    (it, i) => it !== "separator" || (i > 0 && i < visible.length - 1 && visible[i - 1] !== "separator"),
  );

  return createPortal(
    <div
      ref={ref}
      role="menu"
      className="menu-list dbx-ctx"
      style={{ position: "fixed", left: pos.left, top: pos.top }}
      onKeyDown={onKeyDown}
      onContextMenu={(e) => e.preventDefault()}
    >
      {cleaned.map((it, i) =>
        it === "separator" ? (
          <div key={i} className="dbx-ctx-sep" role="separator" />
        ) : (
          <button
            key={i}
            type="button"
            role="menuitem"
            className={`menu-item${it.danger ? " danger" : ""}`}
            disabled={it.disabled}
            onClick={() => {
              onClose();
              it.onSelect();
            }}
          >
            {it.icon}
            <span style={{ flex: 1 }}>{it.label}</span>
            {it.hint ? <span className="dbx-ctx-hint">{it.hint}</span> : null}
          </button>
        ),
      )}
    </div>,
    document.body,
  );
}
