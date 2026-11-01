// Automatic layout of the ER diagram with ELK's layered algorithm. ELK is
// loaded on demand, on the main thread: the dashboard's CSP has no blob:
// workers, and a few hundred cards lay out in well under a second.

export interface LayoutNode {
  id: string;
  width: number;
  height: number;
}

export interface LayoutEdge {
  id: string;
  source: string;
  target: string;
}

export type Positions = Record<string, { x: number; y: number }>;

type ElkInstance = {
  layout: (graph: {
    id: string;
    layoutOptions: Record<string, string>;
    children: { id: string; width: number; height: number; x?: number; y?: number }[];
    edges: { id: string; sources: string[]; targets: string[] }[];
  }) => Promise<{ children?: { id: string; width: number; height: number; x?: number; y?: number }[] }>;
};

let elk: Promise<ElkInstance> | null = null;

function loadElk(): Promise<ElkInstance> {
  elk ??= import("elkjs/lib/elk.bundled.js").then((m) => new m.default() as unknown as ElkInstance);
  return elk;
}

/** From this many tables on, the layered layout gets too tall around hub tables. */
export const STRESS_LAYOUT_FROM = 30;

type ElkChild = LayoutNode & { x?: number; y?: number };

/**
 * layoutGraph places the cards. Small diagrams use ELK's layered algorithm
 * left to right: a referencing table sits left of the table it references, so
 * lines read "child → parent". Larger ones, where a table such as users is
 * referenced by dozens of others and would stack them in one tall column, use
 * the stress algorithm, then remove overlaps and compact.
 */
export async function layoutGraph(nodes: LayoutNode[], edges: LayoutEdge[]): Promise<Positions> {
  if (nodes.length === 0) return {};
  const e = await loadElk();
  const elkEdges = edges
    .filter((ed) => ed.source !== ed.target)
    .map((ed) => ({ id: ed.id, sources: [ed.source], targets: [ed.target] }));
  const run = async (layoutOptions: Record<string, string>, children: ElkChild[]) =>
    (await e.layout({ id: "root", layoutOptions, children: children.map((c) => ({ ...c })), edges: elkEdges })).children ?? [];

  let placed: ElkChild[];
  if (nodes.length < STRESS_LAYOUT_FROM) {
    placed = await run(
      {
        "elk.algorithm": "layered",
        "elk.direction": "RIGHT",
        "elk.layered.spacing.nodeNodeBetweenLayers": "90",
        "elk.spacing.nodeNode": "36",
        "elk.spacing.componentComponent": "60",
        "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
        "elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
        "elk.separateConnectedComponents": "true",
        "elk.aspectRatio": "1.6",
      },
      nodes,
    );
  } else {
    placed = await run({ "elk.algorithm": "stress", "elk.stress.desiredEdgeLength": "320" }, nodes);
    placed = await run({ "elk.algorithm": "sporeOverlap", "elk.spacing.nodeNode": "40" }, placed);
    placed = await run({ "elk.algorithm": "sporeCompaction", "elk.spacing.nodeNode": "40" }, placed);
  }
  const out: Positions = {};
  for (const c of placed) out[c.id] = { x: c.x ?? 0, y: c.y ?? 0 };
  return out;
}
