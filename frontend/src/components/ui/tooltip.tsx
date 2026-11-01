"use client";

import { Info } from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

type Side = "top" | "bottom" | "right";

/**
 * Tooltip shows `content` on hover and keyboard focus, and hides on Esc.
 * It renders in a portal so overflow:hidden containers (tables, the sidebar,
 * the data browser) never clip it. By default the trigger is described by the
 * tooltip (aria-describedby); pass describe={false} when the content only
 * repeats the trigger's accessible name.
 */
export function Tooltip({
  content,
  children,
  side = "top",
  describe = true,
  disabled,
}: {
  content: ReactNode;
  children: ReactNode;
  side?: Side;
  describe?: boolean;
  disabled?: boolean;
}) {
  const id = useId();
  const anchor = useRef<HTMLSpanElement>(null);
  const bubble = useRef<HTMLSpanElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);

  const show = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setOpen(true), 250);
  }, []);
  const hide = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    setOpen(false);
    setPos(null);
  }, []);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && hide();
    window.addEventListener("keydown", onKey);
    window.addEventListener("scroll", hide, true);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", hide, true);
    };
  }, [open, hide]);

  useLayoutEffect(() => {
    if (!open || !anchor.current || !bubble.current) return;
    const a = anchor.current.getBoundingClientRect();
    const b = bubble.current.getBoundingClientRect();
    const gap = 6;
    const vw = window.innerWidth;
    let top: number;
    let left: number;
    if (side === "right") {
      top = a.top + a.height / 2 - b.height / 2;
      left = a.right + gap;
    } else {
      left = a.left + a.width / 2 - b.width / 2;
      top =
        side === "bottom" || a.top - b.height - gap < 4
          ? a.bottom + gap
          : a.top - b.height - gap;
    }
    left = Math.max(6, Math.min(left, vw - b.width - 6));
    setPos({ top, left });
  }, [open, side]);

  return (
    <span
      ref={anchor}
      className="tooltip"
      aria-describedby={describe && open ? id : undefined}
      onPointerEnter={(e) => e.pointerType === "mouse" && !disabled && show()}
      onPointerLeave={hide}
      onFocus={(e) =>
        !disabled &&
        (e.target as HTMLElement).matches(":focus-visible") &&
        show()
      }
      onBlur={hide}
    >
      {children}
      {open && !disabled && typeof document !== "undefined"
        ? createPortal(
            <span
              ref={bubble}
              role="tooltip"
              id={id}
              className="tooltip-bubble"
              style={
                pos
                  ? { top: pos.top, left: pos.left }
                  : { top: -9999, left: -9999 }
              }
            >
              {content}
            </span>,
            document.body,
          )
        : null}
    </span>
  );
}

/** HelpText is an (i) icon that explains a term or field. */
export function HelpText({
  children,
  label = "More information",
}: {
  children: ReactNode;
  label?: string;
}) {
  return (
    <Tooltip content={children}>
      <button type="button" className="help-icon" aria-label={label}>
        <Info size={13} aria-hidden />
      </button>
    </Tooltip>
  );
}
