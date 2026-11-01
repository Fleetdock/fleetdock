import { describe, expect, it } from "vitest";

import { ACTION_CATEGORIES, actionLabel, actorLabel, auditQuery, auditResourceHref, detailLine } from "./audit";
import type { AuditEvent } from "./types";

const ev = (over: Partial<AuditEvent>): AuditEvent => ({
  id: "e1",
  created_at: "2026-11-01T10:00:00Z",
  actor_kind: "user",
  action: "database.delete",
  outcome: "success",
  resource_type: "database",
  metadata: {},
  ...over,
});

describe("audit labels", () => {
  it("names known actions and falls back to the code", () => {
    expect(actionLabel("sql.execute")).toBe("Ran SQL that writes");
    expect(actionLabel("future.thing")).toBe("future.thing");
  });

  it("uses only category prefixes the API understands", () => {
    for (const g of ACTION_CATEGORIES) for (const o of g.options) expect(o.value).toMatch(/^[a-z_]+\.$/);
  });

  it("says who acted", () => {
    expect(actorLabel(ev({ actor_email: "a@x.dev" }))).toBe("a@x.dev");
    expect(actorLabel(ev({ actor_kind: "system" }))).toBe("Fleetdock CLI");
    expect(actorLabel(ev({ actor_kind: "anonymous" }))).toBe("Unknown");
    expect(actorLabel(ev({ actor_user_id: "0123456789" }))).toBe("Deleted user 01234567");
  });

  it("links resources that have a page", () => {
    expect(auditResourceHref(ev({ resource_id: "d1" }))).toBe("/databases/d1");
    expect(auditResourceHref(ev({ resource_type: "role", resource_id: "r1" }))).toBe("/access/roles");
    expect(auditResourceHref(ev({ resource_type: "agent_token", resource_id: "t" }))).toBeNull();
    expect(auditResourceHref(ev({}))).toBeNull();
  });

  it("summarises metadata", () => {
    expect(detailLine(ev({ metadata: { table: "public.orders", statements: 1 } }))).toBe("public.orders · 1 statement");
    expect(detailLine(ev({ metadata: { via: "cli" } }))).toBe("from the command line");
  });
});

describe("auditQuery", () => {
  const now = Date.parse("2026-11-01T12:34:56Z");

  it("defaults to the last 7 days", () => {
    expect(auditQuery({}, now).get("since")).toBe("2026-10-25T12:34:00.000Z");
  });

  it("passes filters and drops the time bound for all time", () => {
    const q = auditQuery({ actor: "u1", action: "user.", outcome: "denied", period: "all", resource_id: "d1" }, now);
    expect(q.toString()).toBe("actor=u1&action=user.&outcome=denied&resource_id=d1");
  });
});
