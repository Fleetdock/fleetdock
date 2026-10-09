"use client";

import { ErrorText, Field } from "@/components/ui";
import type { DataAccessDraft } from "@/lib/data-access";
import { fieldError } from "@/lib/errors";
import type { DataAccess } from "@/lib/types";

const OPTIONS: { mode: DataAccess; label: string; help: string }[] = [
  {
    mode: "admin",
    label: "Admin login",
    help: "Browse data with the admin login above, like pgAdmin or DBeaver. It reaches every database, so only instance administrators can open the table browser and SQL console.",
  },
  {
    mode: "login",
    label: "Dedicated login",
    help: "Use a login you create on the server, e.g. a read-only user. The server's own grants decide what it can do, and users with access to a database can browse it.",
  },
  {
    mode: "managed",
    label: "Fleetdock-managed roles",
    help: "Fleetdock creates a read-only and a read-write role (fleetdock_ro_… / fleetdock_rw_…) per database on first use, confined to that database by the server. Users with access to a database can browse it.",
  },
];

/**
 * DataAccessFields picks the login the table browser, SQL console, exports
 * and imports connect as. `hasStoredLogin` marks an existing dedicated login,
 * whose password may then be left empty.
 */
export function DataAccessFields({
  value,
  onChange,
  error,
  hasStoredLogin,
}: {
  value: DataAccessDraft;
  onChange: (d: DataAccessDraft) => void;
  error: unknown;
  hasStoredLogin?: boolean;
}) {
  const set = (patch: Partial<DataAccessDraft>) => onChange({ ...value, ...patch });
  const current = OPTIONS.find((o) => o.mode === value.mode) ?? OPTIONS[0];

  return (
    <div className="field">
      <label>Data browsing connects as</label>
      <div className="segmented" role="group" aria-label="Data access login">
        {OPTIONS.map((o) => (
          <button key={o.mode} type="button" aria-pressed={value.mode === o.mode} onClick={() => set({ mode: o.mode })}>
            {o.label}
          </button>
        ))}
      </div>
      <ErrorText message={fieldError(error, "data_access")} />
      <p className="muted text-sm" style={{ marginBottom: 0 }}>
        {current.help}
      </p>
      {value.mode === "login" ? (
        <div className="form-grid" style={{ marginTop: ".6rem" }}>
          <Field label="Data username" error={fieldError(error, "data_username")}>
            <input
              className="input"
              value={value.username}
              onChange={(e) => set({ username: e.target.value })}
              placeholder="app_reader"
              autoComplete="off"
              required
            />
          </Field>
          <Field
            label="Data password"
            hint={hasStoredLogin ? "A password is stored. Leave empty to keep it." : "Stored encrypted."}
            error={fieldError(error, "data_password")}
          >
            <input
              className="input"
              type="password"
              value={value.password}
              onChange={(e) => set({ password: e.target.value })}
              autoComplete="new-password"
              required={!hasStoredLogin}
            />
          </Field>
        </div>
      ) : null}
    </div>
  );
}
