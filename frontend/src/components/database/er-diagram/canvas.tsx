"use client";

import "@xyflow/react/dist/style.css";

import {
  Background,
  BackgroundVariant,
  Controls,
  type Edge,
  MarkerType,
  MiniMap,
  type NodeChange,
  ReactFlow,
  ReactFlowProvider,
  getNodesBounds,
  getViewportForBounds,
  useNodesState,
  useReactFlow,
} from "@xyflow/react";
import { Crosshair, Download, LayoutGrid, Search, X } from "lucide-react";
import { useRouter } from "next/navigation";
import { type FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Button, Callout, HelpText, IconButton, Menu, SearchInput, SegmentedControl, Select, Toolbar } from "@/components/ui";
import { loadJSON, save as saveStorage } from "@/lib/data-browser/storage";
import { friendlyError } from "@/lib/errors";
import type { Diagram } from "@/lib/types";

import {
  LARGE_DIAGRAM,
  type SavedPositions,
  buildGraph,
  cardColumns,
  cardSize,
  isSavedPositions,
  mostConnected,
  neighbourhood,
  relationLabel,
  schemasOf,
  usableSaved,
} from "./graph";
import { type Positions, layoutGraph } from "./layout";
import { HEADER_HANDLE, TableNode, type TableNodeType, handleId } from "./table-node";

const nodeTypes = { table: TableNode };
type Mode = "all" | "focus";

const positionsKey = (databaseId: string) => `diagram:${databaseId}`;

export interface CanvasProps {
  databaseId: string;
  databaseName: string;
  data: Diagram;
}

/** ERCanvas draws the diagram; it is loaded on demand with React Flow and ELK. */
export default function ERCanvas(props: CanvasProps) {
  return (
    <ReactFlowProvider>
      <Canvas {...props} />
    </ReactFlowProvider>
  );
}

function tableHref(databaseId: string, id: string, kind: "table" | "view") {
  return `/data?db=${encodeURIComponent(databaseId)}&${kind === "view" ? "view" : "table"}=${encodeURIComponent(id)}`;
}

function Canvas({ databaseId, databaseName, data }: CanvasProps) {
  const router = useRouter();
  const flow = useReactFlow();
  const wrapper = useRef<HTMLDivElement>(null);
  const g = useMemo(() => buildGraph(data), [data]);
  const schemas = useMemo(() => schemasOf(g), [g]);
  const large = g.tables.size > LARGE_DIAGRAM;

  const [mode, setMode] = useState<Mode>(large ? "focus" : "all");
  const [focus, setFocus] = useState<string | null>(() => (large ? mostConnected(g) : null));
  const [depth, setDepth] = useState(1);
  const [schema, setSchema] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [notice, setNotice] = useState<string | null>(null);
  const [layoutError, setLayoutError] = useState<string | null>(null);
  const [positions, setPositions] = useState<Positions | null>(null);
  const [relayout, setRelayout] = useState(0);
  const [nodes, setNodes, onNodesChangeBase] = useNodesState<TableNodeType>([]);

  // The tables on the canvas: everything, or a neighbourhood, narrowed to
  // one PostgreSQL schema.
  const visible = useMemo(() => {
    let ids: string[] =
      mode === "focus" && focus ? [...neighbourhood(g, [focus], depth)] : [...g.tables.keys()];
    if (schema) ids = ids.filter((id) => g.tables.get(id)?.schema === schema || id === focus);
    return ids.sort();
  }, [g, mode, focus, depth, schema]);
  // Only the whole, unfiltered diagram keeps the positions people drag to.
  const persistent = mode === "all" && !schema;
  const visibleKey = visible.join("\n");

  const cards = useMemo(() => {
    const m = new Map<string, { cols: ReturnType<typeof cardColumns>; size: { width: number; height: number } }>();
    for (const id of visible) {
      const t = g.tables.get(id)!;
      const cols = cardColumns(t, g.keyColumns.get(id));
      m.set(id, { cols, size: cardSize(t, cols) });
    }
    return m;
    // visibleKey stands for visible.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [g, visibleKey]);

  const relations = useMemo(() => {
    const on = new Set(visible);
    return g.relations.filter((r) => on.has(r.source) && on.has(r.target));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [g, visibleKey]);

  // Lay out whenever the set of tables changes (or on "Re-layout").
  useEffect(() => {
    let cancelled = false;
    const saved = persistent ? loadJSON<SavedPositions>(positionsKey(databaseId), {}, isSavedPositions) : {};
    if (relayout === 0 && persistent && usableSaved(saved, visible)) {
      setPositions(saved);
      return;
    }
    setLayoutError(null);
    layoutGraph(
      visible.map((id) => ({ id, ...cards.get(id)!.size })),
      relations.map((r) => ({ id: r.id, source: r.source, target: r.target })),
    )
      .then((p) => {
        if (cancelled) return;
        setPositions(p);
        if (persistent && relayout > 0) saveStorage(positionsKey(databaseId), null);
      })
      .catch((err: unknown) => {
        if (!cancelled) setLayoutError(friendlyError(err, "The diagram could not be laid out."));
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [databaseId, visibleKey, persistent, relayout]);

  // What the selection lights up: its relations and the columns they join.
  const active = useMemo(() => {
    const tables = new Set<string>();
    const columns = new Map<string, Set<string>>();
    const rels = new Set<string>();
    if (!selected) return { tables, columns, rels };
    tables.add(selected);
    const add = (t: string, cols: string[]) => {
      let s = columns.get(t);
      if (!s) columns.set(t, (s = new Set()));
      for (const c of cols) s.add(c);
    };
    for (const r of relations) {
      if (r.source !== selected && r.target !== selected) continue;
      rels.add(r.id);
      tables.add(r.source).add(r.target);
      add(r.source, r.columns);
      add(r.target, r.refColumns);
    }
    return { tables, columns, rels };
  }, [selected, relations]);

  const decorate = useCallback(
    (id: string) => {
      const t = g.tables.get(id)!;
      return {
        table: t,
        cols: cards.get(id)!.cols,
        keys: g.keyColumns.get(id) ?? new Set<string>(),
        href: tableHref(databaseId, id, t.kind),
        activeColumns: active.columns.get(id) ?? new Set<string>(),
        highlighted: id === focus && mode === "focus",
        dimmed: selected !== null && !active.tables.has(id),
      };
    },
    [g, cards, databaseId, active, focus, mode, selected],
  );

  // New positions: rebuild the nodes. Fit once they are measured.
  useEffect(() => {
    if (!positions) return;
    setNodes(
      visible
        .filter((id) => positions[id])
        .map((id) => ({
          id,
          type: "table" as const,
          position: positions[id],
          data: decorate(id),
          selected: id === selected,
          ariaLabel: `Table ${id}`,
        })),
    );
    requestAnimationFrame(() => void flow.fitView({ padding: 0.12, maxZoom: 1.1, duration: 0 }));
    // decorate/selected are applied by the effect below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [positions]);

  // Selection, focus and card contents changed: refresh the node data. Cards
  // that left the canvas go now; the new layout adds the ones that joined.
  useEffect(() => {
    setNodes((nds) => nds.filter((n) => cards.has(n.id)).map((n) => ({ ...n, data: decorate(n.id) })));
  }, [decorate, cards, setNodes]);

  const edges = useMemo<Edge[]>(() => {
    const shown = (table: string, col: string | undefined) =>
      col !== undefined && cards.get(table)?.cols.shown.some((c) => c.name === col);
    return relations.map((r) => {
      const on = active.rels.has(r.id);
      return {
        id: r.id,
        source: r.source,
        target: r.target,
        sourceHandle: handleId(shown(r.source, r.columns[0]) ? r.columns[0] : HEADER_HANDLE, "s"),
        targetHandle: handleId(shown(r.target, r.refColumns[0]) ? r.refColumns[0] : HEADER_HANDLE, "t"),
        type: "smoothstep",
        markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16 },
        className: [
          "erd-edge",
          r.optional && "erd-edge-optional",
          on && "is-active",
          selected !== null && !on && "is-dimmed",
        ]
          .filter(Boolean)
          .join(" "),
        ariaLabel: relationLabel(r),
        zIndex: on ? 1 : 0,
        focusable: false,
      } satisfies Edge;
    });
  }, [relations, cards, active, selected]);

  const onNodesChange = useCallback(
    (changes: NodeChange<TableNodeType>[]) => {
      onNodesChangeBase(changes);
      for (const c of changes) {
        if (c.type !== "select") continue;
        if (c.selected) setSelected(c.id);
        else setSelected((s) => (s === c.id ? null : s));
      }
    },
    [onNodesChangeBase],
  );

  const onDragStop = useCallback(() => {
    if (!persistent) return;
    const out: SavedPositions = {};
    for (const n of flow.getNodes()) out[n.id] = { x: Math.round(n.position.x), y: Math.round(n.position.y) };
    saveStorage(positionsKey(databaseId), JSON.stringify(out));
  }, [persistent, flow, databaseId]);

  const centerOn = useCallback(
    (id: string) => {
      setSelected(id);
      setNodes((nds) => nds.map((n) => ({ ...n, selected: n.id === id })));
      requestAnimationFrame(() => void flow.fitView({ nodes: [{ id }], maxZoom: 1.2, duration: 300, padding: 0.4 }));
    },
    [flow, setNodes],
  );

  const onSearch = (e: FormEvent) => {
    e.preventDefault();
    const q = query.trim().toLowerCase();
    if (!q) return;
    const ids = [...g.tables.keys()];
    const hit =
      ids.find((id) => id.toLowerCase() === q) ??
      ids.find((id) => g.tables.get(id)!.name.toLowerCase() === q) ??
      ids.find((id) => id.toLowerCase().includes(q));
    if (!hit) {
      setNotice(`No table matches “${query.trim()}”.`);
      return;
    }
    setNotice(null);
    if (mode === "focus") {
      setFocus(hit);
      setSelected(hit);
    } else if (!visible.includes(hit)) {
      setSchema("");
      setSelected(hit);
    } else {
      centerOn(hit);
    }
  };

  const exportImage = async (format: "png" | "svg") => {
    const el = wrapper.current?.querySelector<HTMLElement>(".react-flow__viewport");
    if (!el || nodes.length === 0) return;
    const { toPng, toSvg } = await import("html-to-image");
    const bounds = getNodesBounds(nodes);
    const pad = 40;
    const scale = Math.min(1, 8000 / Math.max(bounds.width + pad * 2, bounds.height + pad * 2));
    const width = Math.round((bounds.width + pad * 2) * scale);
    const height = Math.round((bounds.height + pad * 2) * scale);
    const vp = getViewportForBounds(bounds, width, height, scale, scale, pad * scale);
    const background = getComputedStyle(wrapper.current!).getPropertyValue("--panel").trim() || "white";
    const opts = {
      backgroundColor: background,
      width,
      height,
      style: { width: `${width}px`, height: `${height}px`, transform: `translate(${vp.x}px, ${vp.y}px) scale(${vp.zoom})` },
    };
    try {
      const url = format === "png" ? await toPng(el, opts) : await toSvg(el, opts);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${databaseName}-diagram.${format}`;
      a.click();
    } catch (err) {
      setNotice(friendlyError(err, "The image could not be created."));
    }
  };

  const [dismissed, setDismissed] = useState<string[]>([]);
  const notices = [
    data.truncated
      ? { id: "truncated", tone: "warning" as const, dismissible: true, text: "Only part of this database is shown: it has more tables, columns or foreign keys than the diagram reads at once." }
      : null,
    large && mode === "focus"
      ? { id: "large", tone: "info" as const, dismissible: true, text: `This database has ${g.tables.size} tables, so the diagram starts with one table and its neighbours. Search for a table to move the focus, or show all tables.` }
      : null,
    notice ? { id: `notice:${notice}`, tone: "warning" as const, dismissible: true, text: notice } : null,
    layoutError ? { id: "layout", tone: "danger" as const, dismissible: false, text: layoutError } : null,
  ].filter((n): n is NonNullable<typeof n> => n !== null && !dismissed.includes(n.id));

  const counts = `${visible.length} of ${g.tables.size} table${g.tables.size === 1 ? "" : "s"} · ${relations.length} relation${relations.length === 1 ? "" : "s"}`;
  const tableOptions = useMemo(() => [...g.tables.keys()].sort(), [g]);

  return (
    <div className="erd">
      <Toolbar className="erd-toolbar">
        <form onSubmit={onSearch} role="search" className="erd-search">
          <SearchInput
            icon={<Search size={15} aria-hidden />}
            placeholder="Find a table"
            aria-label="Find a table"
            list={`erd-tables-${databaseId}`}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setNotice(null);
            }}
          />
          <datalist id={`erd-tables-${databaseId}`}>
            {tableOptions.slice(0, 500).map((id) => (
              <option key={id} value={id} />
            ))}
          </datalist>
        </form>
        <SegmentedControl<Mode>
          label="Tables to show"
          size="sm"
          value={mode}
          onChange={(m) => {
            if (m === "focus" && !focus) setFocus(selected ?? mostConnected(g));
            setMode(m);
          }}
          options={[
            { value: "all", label: "All tables" },
            { value: "focus", label: "Neighbours" },
          ]}
        />
        {mode === "focus" ? (
          <Select size="sm" className="w-auto" aria-label="Neighbour depth" value={depth} onChange={(e) => setDepth(Number(e.target.value))}>
            <option value={1}>1 step</option>
            <option value={2}>2 steps</option>
            <option value={3}>3 steps</option>
          </Select>
        ) : null}
        {schemas.length > 1 ? (
          <Select size="sm" className="w-auto" aria-label="Schema" value={schema} onChange={(e) => setSchema(e.target.value)}>
            <option value="">All schemas</option>
            {schemas.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </Select>
        ) : null}
        <div className="toolbar-spacer" />
        <span className="text-xs text-muted num">{counts}</span>
        <HelpText label="How to read the diagram">
          Arrows point from the referencing table to the one it references; dashed lines are optional (nullable)
          keys. Click a table to highlight its relations, double-click it to open it in the data browser.
          {persistent ? " Positions you drag are kept in this browser." : ""}
        </HelpText>
        {selected && (mode === "all" || selected !== focus) ? (
          <Button
            size="sm"
            variant="ghost"
            icon={<Crosshair size={15} aria-hidden />}
            onClick={() => {
              setFocus(selected);
              setMode("focus");
            }}
          >
            Neighbours of {g.tables.get(selected)?.name ?? selected}
          </Button>
        ) : null}
        <Button size="sm" variant="secondary" icon={<LayoutGrid size={15} aria-hidden />} onClick={() => setRelayout((n) => n + 1)}>
          Re-layout
        </Button>
        <Menu
          label="Export diagram"
          trigger={<Download size={15} aria-hidden />}
          items={[
            { label: "Download PNG", onSelect: () => void exportImage("png") },
            { label: "Download SVG", onSelect: () => void exportImage("svg") },
          ]}
        />
      </Toolbar>

      {notices.length > 0 ? (
        <div className="erd-notices">
          {notices.map((n) => (
            <Callout
              key={n.id}
              tone={n.tone}
              actions={
                n.dismissible ? (
                  <IconButton label="Dismiss" onClick={() => setDismissed((d) => [...d, n.id])}>
                    <X size={15} aria-hidden />
                  </IconButton>
                ) : undefined
              }
            >
              {n.text}
            </Callout>
          ))}
        </div>
      ) : null}
      <div ref={wrapper} className="erd-canvas" aria-label={`Entity-relationship diagram of ${databaseName}`}>
        <ReactFlow<TableNodeType, Edge>
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onNodeDragStop={onDragStop}
          onNodeDoubleClick={(_, n) => router.push(n.data.href)}
          onPaneClick={() => setSelected(null)}
          nodesConnectable={false}
          edgesFocusable={false}
          minZoom={0.05}
          maxZoom={2}
          onlyRenderVisibleElements={nodes.length > 80}
          proOptions={{ hideAttribution: true }}
          fitView
        >
          <Background variant={BackgroundVariant.Dots} gap={18} size={1} />
          <Controls showInteractive={false} />
          {nodes.length > 12 ? <MiniMap pannable zoomable className="erd-minimap" /> : null}
        </ReactFlow>
      </div>
    </div>
  );
}
