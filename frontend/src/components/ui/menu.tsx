"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { MoreHorizontal } from "lucide-react";

export type MenuItem = {
  label: ReactNode;
  icon?: ReactNode;
  onSelect: () => void;
  danger?: boolean;
  disabled?: boolean;
  hidden?: boolean;
};

/**
 * Menu is a dropdown of actions (row actions, "more" menus). Keyboard: Enter
 * or Space opens it, arrow keys move, Esc closes and returns focus.
 */
export function Menu({
  items,
  label = "More actions",
  trigger,
  align = "right",
}: {
  items: MenuItem[];
  label?: string;
  trigger?: ReactNode;
  align?: "left" | "right";
}) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const visible = items.filter((i) => !i.hidden);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    root.current?.querySelector<HTMLElement>('[role="menuitem"]:not([disabled])')?.focus();
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  if (visible.length === 0) return null;

  function onKeyDown(e: React.KeyboardEvent) {
    const nodes = Array.from(root.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])') ?? []);
    const i = nodes.indexOf(document.activeElement as HTMLElement);
    if (e.key === "Escape") {
      e.stopPropagation();
      setOpen(false);
      button.current?.focus();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      nodes[(i + 1) % nodes.length]?.focus();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      nodes[(i - 1 + nodes.length) % nodes.length]?.focus();
    } else if (e.key === "Tab") {
      setOpen(false);
    }
  }

  return (
    <div className="menu" ref={root} onKeyDown={onKeyDown}>
      <button
        ref={button}
        type="button"
        className="btn btn-ghost btn-sm"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={id}
        aria-label={trigger ? undefined : label}
        onClick={() => setOpen((v) => !v)}
      >
        {trigger ?? <MoreHorizontal size={16} aria-hidden />}
      </button>
      {open ? (
        <div id={id} role="menu" className={`menu-list menu-${align}`}>
          {visible.map((item, i) => (
            <button
              key={i}
              type="button"
              role="menuitem"
              className={`menu-item${item.danger ? " danger" : ""}`}
              disabled={item.disabled}
              onClick={() => {
                setOpen(false);
                item.onSelect();
              }}
            >
              {item.icon}
              {item.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
