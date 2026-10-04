"use client";

import { useId, type ReactNode } from "react";
import { Info } from "lucide-react";

/**
 * Tooltip shows `content` on hover and keyboard focus. The trigger is
 * described by the tooltip (aria-describedby), so screen readers read it too.
 */
export function Tooltip({ content, children }: { content: ReactNode; children: ReactNode }) {
  const id = useId();
  return (
    <span className="tooltip" aria-describedby={id}>
      {children}
      <span role="tooltip" id={id} className="tooltip-bubble">
        {content}
      </span>
    </span>
  );
}

/** HelpText is an (i) icon that explains a term or field. */
export function HelpText({ children, label = "More information" }: { children: ReactNode; label?: string }) {
  return (
    <Tooltip content={children}>
      <button type="button" className="help-icon" aria-label={label}>
        <Info size={13} aria-hidden />
      </button>
    </Tooltip>
  );
}
