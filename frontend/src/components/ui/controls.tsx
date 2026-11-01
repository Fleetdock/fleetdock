"use client";

import { AlertTriangle, CheckCircle2, Info, XCircle } from "lucide-react";
import Link from "next/link";
import {
  forwardRef,
  type ButtonHTMLAttributes,
  type ComponentProps,
  type HTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TableHTMLAttributes,
  type TextareaHTMLAttributes,
} from "react";

import { Tooltip } from "./tooltip";

/** cx joins class names, skipping falsy ones. */
export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(" ");
}

// ---------------------------------------------------------------------------
// Buttons

export type ButtonVariant =
  | "primary"
  | "secondary"
  | "ghost"
  | "danger"
  | "danger-solid";
export type ButtonSize = "sm" | "md" | "lg";

type ButtonStyle = {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Show a spinner and disable the button. */
  loading?: boolean;
  /** Leading icon. */
  icon?: ReactNode;
  block?: boolean;
  active?: boolean;
};

export function buttonClass(
  {
    variant = "secondary",
    size = "md",
    block,
    active,
  }: Omit<ButtonStyle, "loading" | "icon"> = {},
  extra?: string,
): string {
  return cx(
    "btn",
    variant === "primary" && "btn-primary",
    variant === "ghost" && "btn-ghost",
    variant === "danger" && "btn-danger",
    variant === "danger-solid" && "btn-danger-solid",
    size === "sm" && "btn-sm",
    size === "lg" && "btn-lg",
    block && "btn-block",
    active && "btn-active",
    extra,
  );
}

export type ButtonProps = ButtonStyle & ButtonHTMLAttributes<HTMLButtonElement>;

/** Button is the standard action control. Defaults to type="button". */
export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  function Button(
    {
      variant,
      size,
      loading,
      icon,
      block,
      active,
      className,
      children,
      disabled,
      type = "button",
      ...rest
    },
    ref,
  ) {
    return (
      <button
        ref={ref}
        type={type}
        className={buttonClass({ variant, size, block, active }, className)}
        disabled={disabled || loading}
        aria-busy={loading || undefined}
        {...rest}
      >
        {loading ? <span className="spin" aria-hidden /> : icon}
        {children}
      </button>
    );
  },
);

export type ButtonLinkProps = ButtonStyle & ComponentProps<typeof Link>;

/** ButtonLink is a link styled as a button — for actions that navigate. */
export function ButtonLink({
  variant,
  size,
  icon,
  block,
  active,
  className,
  children,
  ...rest
}: ButtonLinkProps) {
  return (
    <Link
      className={buttonClass({ variant, size, block, active }, className)}
      {...rest}
    >
      {icon}
      {children}
    </Link>
  );
}

export type IconButtonProps = Omit<
  ButtonProps,
  "children" | "icon" | "block"
> & {
  /** Accessible name; also shown as a tooltip. */
  label: string;
  children: ReactNode;
  /** Hide the tooltip (when a visible label is next to the button). */
  noTooltip?: boolean;
};

/** IconButton is a square, icon-only button. `label` is required for screen readers. */
export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  function IconButton(
    {
      label,
      children,
      variant = "ghost",
      size = "sm",
      noTooltip,
      className,
      ...rest
    },
    ref,
  ) {
    const btn = (
      <Button
        ref={ref}
        variant={variant}
        size={size}
        aria-label={label}
        className={cx("btn-icon", size === "md" && "btn-md", className)}
        {...rest}
      >
        {children}
      </Button>
    );
    return noTooltip ? (
      btn
    ) : (
      <Tooltip content={label} describe={false}>
        {btn}
      </Tooltip>
    );
  },
);

// ---------------------------------------------------------------------------
// Form controls

type ControlSize = { size?: "sm" | "md" };

export const Input = forwardRef<
  HTMLInputElement,
  Omit<InputHTMLAttributes<HTMLInputElement>, "size"> & ControlSize
>(function Input({ className, size, ...rest }, ref) {
  return (
    <input
      ref={ref}
      className={cx("input", size === "sm" && "input-sm", className)}
      {...rest}
    />
  );
});

export const Textarea = forwardRef<
  HTMLTextAreaElement,
  TextareaHTMLAttributes<HTMLTextAreaElement>
>(function Textarea({ className, ...rest }, ref) {
  return <textarea ref={ref} className={cx("input", className)} {...rest} />;
});

export const Select = forwardRef<
  HTMLSelectElement,
  Omit<SelectHTMLAttributes<HTMLSelectElement>, "size"> & ControlSize
>(function Select({ className, size, children, ...rest }, ref) {
  return (
    <select
      ref={ref}
      className={cx("input", size === "sm" && "input-sm", className)}
      {...rest}
    >
      {children}
    </select>
  );
});

/** Checkbox with its label (and optional hint) — the whole row is clickable. */
export function Checkbox({
  label,
  hint,
  className,
  ...rest
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & {
  label: ReactNode;
  hint?: ReactNode;
}) {
  return (
    <label className={cx("check", className)}>
      <input type="checkbox" {...rest} />
      <span>
        {label}
        {hint ? <span className="check-hint">{hint}</span> : null}
      </span>
    </label>
  );
}

/** Switch is an on/off toggle backed by a checkbox (role="switch"). */
export function Switch({
  label,
  checked,
  onCheckedChange,
  disabled,
  className,
}: {
  label: ReactNode;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
}) {
  return (
    <label className={cx("switch", className)}>
      <input
        type="checkbox"
        role="switch"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onCheckedChange(e.target.checked)}
      />
      <span className="switch-track" aria-hidden />
      <span>{label}</span>
    </label>
  );
}

/** SearchInput is a text input with a leading search icon. */
export function SearchInput({
  icon,
  className,
  ...rest
}: Omit<InputHTMLAttributes<HTMLInputElement>, "size" | "type"> & {
  icon: ReactNode;
}) {
  return (
    <div className={cx("search-box", className)}>
      {icon}
      <input type="search" className="input" {...rest} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Surfaces

export function Card({ className, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx("card", className)} {...rest} />;
}

export function CardHeader({
  title,
  icon,
  actions,
  description,
  className,
}: {
  title: ReactNode;
  icon?: ReactNode;
  actions?: ReactNode;
  description?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cx("card-header", className)}>
      <div className="min-w-0">
        <h2 className="card-title">
          {icon}
          {title}
        </h2>
        {description ? (
          <p className="mt-0.5 text-xs muted">{description}</p>
        ) : null}
      </div>
      {actions ? <div className="row">{actions}</div> : null}
    </div>
  );
}

export function CardBody({
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx("card-body", className)} {...rest} />;
}

export function CardFooter({
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx("card-footer", className)} {...rest} />;
}

// ---------------------------------------------------------------------------
// Badges and callouts

export type Tone =
  | "neutral"
  | "success"
  | "warning"
  | "danger"
  | "info"
  | "accent";

const TONE_CLASS: Record<Tone, string> = {
  neutral: "badge-gray",
  success: "badge-green",
  warning: "badge-amber",
  danger: "badge-red",
  info: "badge-blue",
  accent: "badge-teal",
};

export function Badge({
  tone = "neutral",
  dot,
  pulse,
  className,
  children,
  ...rest
}: HTMLAttributes<HTMLSpanElement> & {
  tone?: Tone;
  dot?: boolean;
  pulse?: boolean;
}) {
  return (
    <span
      className={cx(
        "badge",
        TONE_CLASS[tone],
        pulse && "badge-pulse",
        className,
      )}
      {...rest}
    >
      {dot ? <span className="dot" aria-hidden /> : null}
      {children}
    </span>
  );
}

const CALLOUT_ICON = {
  info: Info,
  warning: AlertTriangle,
  danger: XCircle,
  success: CheckCircle2,
} as const;

/** Callout is an inline message: info, warning, danger or success. */
export function Callout({
  tone = "info",
  title,
  children,
  actions,
  className,
  icon = true,
  role,
}: {
  tone?: keyof typeof CALLOUT_ICON;
  title?: ReactNode;
  children?: ReactNode;
  actions?: ReactNode;
  className?: string;
  /** false hides the icon. */
  icon?: boolean;
  role?: "alert" | "status";
}) {
  const Icon = CALLOUT_ICON[tone];
  return (
    <div
      className={cx("callout", `callout-${tone}`, className)}
      role={role ?? (tone === "danger" ? "alert" : undefined)}
    >
      {icon ? <Icon size={16} aria-hidden /> : null}
      <div className="callout-body">
        {title ? <p className="callout-title">{title}</p> : null}
        {children}
      </div>
      {actions ? <div className="row">{actions}</div> : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Segmented control

export type SegmentOption<T extends string> = {
  value: T;
  label: ReactNode;
  hidden?: boolean;
};

/**
 * SegmentedControl picks one of a few options (a range, a filter). Buttons
 * carry aria-pressed so assistive tech reads the selected one.
 */
export function SegmentedControl<T extends string>({
  options,
  value,
  onChange,
  label,
  size = "md",
  className,
}: {
  options: SegmentOption<T>[];
  value: T;
  onChange: (value: T) => void;
  label: string;
  size?: "sm" | "md";
  className?: string;
}) {
  return (
    <div
      className={cx("segmented", size === "sm" && "segmented-sm", className)}
      role="group"
      aria-label={label}
    >
      {options
        .filter((o) => !o.hidden)
        .map((o) => (
          <button
            key={o.value}
            type="button"
            aria-pressed={o.value === value}
            onClick={() => onChange(o.value)}
          >
            {o.label}
          </button>
        ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tables and layout helpers

/** Table is a styled <table> in a horizontally scrolling card. */
export function Table({
  className,
  compact,
  sticky,
  bare,
  wrapClassName,
  ...rest
}: TableHTMLAttributes<HTMLTableElement> & {
  compact?: boolean;
  sticky?: boolean;
  bare?: boolean;
  wrapClassName?: string;
}) {
  return (
    <div className={cx(!bare && "card", "table-wrap", wrapClassName)}>
      <table
        className={cx(
          "table",
          compact && "table-compact",
          sticky && "table-sticky",
          className,
        )}
        {...rest}
      />
    </div>
  );
}

/** Toolbar is a wrapping row of filters and actions above a list. */
export function Toolbar({
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx("toolbar", className)} {...rest} />;
}
