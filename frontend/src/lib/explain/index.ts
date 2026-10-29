import type { ExplainResult } from "../types";
import { parseMySQLPlan, parseMySQLTree } from "./mysql";
import { parsePostgresPlan } from "./postgres";
import type { Plan } from "./types";

export type { Plan, PlanNode } from "./types";
export { walk } from "./types";

/** parsePlan turns an /explain response into the common plan tree. */
export function parsePlan(res: ExplainResult): Plan {
  switch (res.format) {
    case "postgres":
      return parsePostgresPlan(res.plan, res.analyzed);
    case "mysql":
    case "mariadb":
      return parseMySQLPlan(res.plan, res.analyzed);
    case "mysql-tree":
      return parseMySQLTree(res.text ?? "");
    default:
      throw new Error(`unknown plan format ${String(res.format)}`);
  }
}
