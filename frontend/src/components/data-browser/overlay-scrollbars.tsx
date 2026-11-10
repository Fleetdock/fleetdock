"use client";

import { useEffect, useRef, type RefObject } from "react";

// Smallest thumb length in px, so very long tables still give a grabbable thumb.
const MIN_THUMB = 28;

type Axis = "x" | "y";

/**
 * OverlayScrollbars draws thin scrollbars over a scroller whose native bars are
 * hidden, so they take no space. Render it beside the scroller in .dbx-grid-frame.
 */
export function OverlayScrollbars({ target }: { target: RefObject<HTMLElement | null> }) {
  const bars = useRef<Record<Axis, HTMLDivElement | null>>({ x: null, y: null });
  const thumbs = useRef<Record<Axis, HTMLDivElement | null>>({ x: null, y: null });

  useEffect(() => {
    const el = target.current;
    if (!el) return;
    const frame = el.parentElement;
    let raf = 0;
    let idle: ReturnType<typeof setTimeout> | undefined;

    const update = () => {
      raf = 0;
      const hasX = el.scrollWidth > el.clientWidth + 1;
      const hasY = el.scrollHeight > el.clientHeight + 1;
      frame?.classList.toggle("has-both-bars", hasX && hasY);
      for (const axis of ["x", "y"] as const) {
        const bar = bars.current[axis];
        const thumb = thumbs.current[axis];
        if (!bar || !thumb) continue;
        const on = axis === "x" ? hasX : hasY;
        bar.hidden = !on;
        if (!on) continue;
        const view = axis === "x" ? el.clientWidth : el.clientHeight;
        const content = axis === "x" ? el.scrollWidth : el.scrollHeight;
        const pos = axis === "x" ? el.scrollLeft : el.scrollTop;
        const track = axis === "x" ? bar.clientWidth : bar.clientHeight;
        const len = Math.max(MIN_THUMB, (track * view) / content);
        const offset = ((track - len) * pos) / (content - view);
        thumb.style[axis === "x" ? "width" : "height"] = `${len}px`;
        thumb.style.transform = axis === "x" ? `translateX(${offset}px)` : `translateY(${offset}px)`;
      }
    };
    const schedule = () => {
      if (!raf) raf = requestAnimationFrame(update);
    };
    // Show the bars briefly while scrolling (touch, trackpads, keyboard).
    const onScroll = () => {
      schedule();
      frame?.classList.add("is-scrolling");
      clearTimeout(idle);
      idle = setTimeout(() => frame?.classList.remove("is-scrolling"), 900);
    };

    el.addEventListener("scroll", onScroll, { passive: true });
    const ro = new ResizeObserver(schedule);
    ro.observe(el);
    // The content (table, virtual spacers, empty state) changes the scroll size.
    const watchChildren = () => {
      for (const child of Array.from(el.children)) ro.observe(child);
    };
    watchChildren();
    const mo = new MutationObserver(() => {
      watchChildren();
      schedule();
    });
    mo.observe(el, { childList: true });
    update();

    return () => {
      el.removeEventListener("scroll", onScroll);
      ro.disconnect();
      mo.disconnect();
      cancelAnimationFrame(raf);
      clearTimeout(idle);
    };
  }, [target]);

  function onThumbDown(e: React.PointerEvent<HTMLDivElement>, axis: Axis) {
    const el = target.current;
    const bar = bars.current[axis];
    if (!el || !bar || e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    const thumb = e.currentTarget;
    thumb.setPointerCapture(e.pointerId);
    const start = axis === "x" ? e.clientX : e.clientY;
    const startPos = axis === "x" ? el.scrollLeft : el.scrollTop;
    const view = axis === "x" ? el.clientWidth : el.clientHeight;
    const content = axis === "x" ? el.scrollWidth : el.scrollHeight;
    const track = axis === "x" ? bar.clientWidth : bar.clientHeight;
    const len = axis === "x" ? thumb.offsetWidth : thumb.offsetHeight;
    const ratio = (content - view) / Math.max(1, track - len);
    bar.classList.add("is-dragging");
    const onMove = (ev: PointerEvent) => {
      const delta = ((axis === "x" ? ev.clientX : ev.clientY) - start) * ratio;
      if (axis === "x") el.scrollLeft = startPos + delta;
      else el.scrollTop = startPos + delta;
    };
    const onUp = () => {
      bar.classList.remove("is-dragging");
      thumb.removeEventListener("pointermove", onMove);
      thumb.removeEventListener("pointerup", onUp);
      thumb.removeEventListener("pointercancel", onUp);
    };
    thumb.addEventListener("pointermove", onMove);
    thumb.addEventListener("pointerup", onUp);
    thumb.addEventListener("pointercancel", onUp);
  }

  // A click on the track pages one viewport toward the pointer.
  function onTrackDown(e: React.PointerEvent<HTMLDivElement>, axis: Axis) {
    const el = target.current;
    const thumb = thumbs.current[axis];
    if (!el || !thumb || e.button !== 0) return;
    e.preventDefault();
    const r = thumb.getBoundingClientRect();
    const before = axis === "x" ? e.clientX < r.left : e.clientY < r.top;
    const page = (axis === "x" ? el.clientWidth : el.clientHeight) * 0.9;
    el.scrollBy(axis === "x" ? { left: before ? -page : page } : { top: before ? -page : page });
  }

  return (
    <>
      {(["y", "x"] as const).map((axis) => (
        <div
          key={axis}
          ref={(n) => {
            bars.current[axis] = n;
          }}
          className={`dbx-scrollbar is-${axis}`}
          onPointerDown={(e) => onTrackDown(e, axis)}
          // Keep focus (and the cell cursor) in the grid.
          onMouseDown={(e) => e.preventDefault()}
          aria-hidden
          hidden
        >
          <div
            ref={(n) => {
              thumbs.current[axis] = n;
            }}
            className="dbx-scrollbar-thumb"
            onPointerDown={(e) => onThumbDown(e, axis)}
          />
        </div>
      ))}
    </>
  );
}
