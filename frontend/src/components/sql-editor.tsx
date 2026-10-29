"use client";

import { indentWithTab } from "@codemirror/commands";
import { MariaSQL, MySQL, PostgreSQL, sql } from "@codemirror/lang-sql";
import { type Extension } from "@codemirror/state";
import { EditorView, keymap, lineNumbers, placeholder } from "@codemirror/view";
import CodeMirror from "@uiw/react-codemirror";
import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";

import type { SqlCompletion } from "@/lib/sql-schema";

function useIsDark() {
  const [dark, setDark] = useState(false);

  useEffect(() => {
    const el = document.documentElement;
    const update = () => setDark(el.classList.contains("dark"));
    update();
    const obs = new MutationObserver(update);
    obs.observe(el, { attributes: true, attributeFilter: ["class"] });
    return () => obs.disconnect();
  }, []);

  return dark;
}

function fleetdockEditorTheme(dark: boolean): Extension {
  return EditorView.theme(
    {
      "&": {
        backgroundColor: "var(--panel)",
        color: "var(--fg)",
        fontSize: "13px",
      },
      ".cm-scroller": {
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
        lineHeight: "1.5",
      },
      ".cm-content": {
        caretColor: "var(--accent)",
        padding: "10px 0",
      },
      ".cm-cursor, .cm-dropCursor": {
        borderLeftColor: "var(--accent)",
      },
      "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection": {
        backgroundColor: dark ? "color-mix(in srgb, var(--accent) 28%, transparent)" : "color-mix(in srgb, var(--accent) 18%, transparent)",
      },
      ".cm-activeLine": {
        backgroundColor: dark ? "color-mix(in srgb, var(--panel-2) 80%, transparent)" : "color-mix(in srgb, var(--panel-2) 65%, transparent)",
      },
      ".cm-gutters": {
        backgroundColor: "var(--panel-2)",
        color: "var(--muted)",
        border: "none",
        borderRight: "1px solid var(--border)",
      },
      ".cm-activeLineGutter": {
        backgroundColor: dark ? "color-mix(in srgb, var(--panel-2) 90%, transparent)" : "var(--panel-2)",
        color: "var(--fg)",
      },
      ".cm-lineNumbers .cm-gutterElement": {
        padding: "0 8px 0 4px",
        minWidth: "2.25rem",
      },
      ".cm-tooltip": {
        backgroundColor: "var(--panel)",
        color: "var(--fg)",
        border: "1px solid var(--border)",
        borderRadius: "6px",
        boxShadow: "0 6px 20px rgb(0 0 0 / 0.12)",
        overflow: "hidden",
      },
      ".cm-tooltip.cm-tooltip-autocomplete > ul": {
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
        fontSize: "12.5px",
        maxHeight: "16em",
      },
      ".cm-tooltip.cm-tooltip-autocomplete > ul > li": {
        padding: "2px 10px 2px 6px",
      },
      ".cm-tooltip-autocomplete ul li[aria-selected]": {
        backgroundColor: dark ? "color-mix(in srgb, var(--accent) 35%, transparent)" : "color-mix(in srgb, var(--accent) 16%, transparent)",
        color: "var(--fg)",
      },
      ".cm-completionDetail": {
        color: "var(--muted)",
        fontStyle: "normal",
        marginLeft: "1em",
      },
      ".cm-completionMatchedText": {
        textDecoration: "none",
        fontWeight: 600,
        color: "var(--accent)",
      },
    },
    { dark },
  );
}

function syntaxTheme(dark: boolean): Extension {
  return EditorView.theme(
    {
      ".cm-keyword": { color: dark ? "#7dd3fc" : "#0369a1", fontWeight: 500 },
      ".cm-string": { color: dark ? "#86efac" : "#15803d" },
      ".cm-number": { color: dark ? "#fcd34d" : "#b45309" },
      ".cm-comment": { color: "var(--muted)", fontStyle: "italic" },
      ".cm-operator": { color: dark ? "#f9a8d4" : "#be185d" },
      ".cm-variableName": { color: "var(--fg)" },
      ".cm-typeName": { color: dark ? "#a5b4fc" : "#4338ca" },
      ".cm-propertyName": { color: dark ? "#93c5fd" : "#1d4ed8" },
    },
    { dark },
  );
}

export function getQueryToRun(view: EditorView): string {
  const { from, to } = view.state.selection.main;
  const text = from !== to ? view.state.sliceDoc(from, to) : view.state.doc.toString();
  return text.trim();
}

export interface SqlEditorHandle {
  getQueryToRun: () => string;
  focus: () => void;
}

type SqlEditorProps = {
  value: string;
  onChange: (value: string) => void;
  onRun?: () => void;
  placeholder?: string;
  minHeight?: string;
  /** Show the text without letting it be changed (object definitions). */
  readOnly?: boolean;
  /** Engine of the database, for its keywords and identifier quoting. */
  engine?: string;
  /** Tables and columns to autocomplete; keywords complete without it. */
  completion?: SqlCompletion;
};

function dialectFor(engine: string | undefined) {
  switch (engine) {
    case "postgres":
      return PostgreSQL;
    case "mariadb":
      return MariaSQL;
    case "mysql":
      return MySQL;
    default:
      return undefined;
  }
}

export const SqlEditor = forwardRef<SqlEditorHandle, SqlEditorProps>(function SqlEditor(
  {
    value,
    onChange,
    onRun,
    placeholder: placeholderText = "SELECT * FROM ...",
    minHeight = "200px",
    readOnly = false,
    engine,
    completion,
  },
  ref,
) {
  const dark = useIsDark();
  const viewRef = useRef<EditorView | null>(null);

  useImperativeHandle(ref, () => ({
    getQueryToRun: () => (viewRef.current ? getQueryToRun(viewRef.current) : value.trim()),
    focus: () => viewRef.current?.focus(),
  }));

  const extensions = useMemo(() => {
    return [
      sql({
        dialect: dialectFor(engine),
        schema: completion?.schema,
        defaultSchema: completion?.defaultSchema,
        upperCaseKeywords: true,
      }),
      lineNumbers(),
      EditorView.lineWrapping,
      placeholder(placeholderText),
      keymap.of([
        indentWithTab,
        {
          key: "Mod-Enter",
          run() {
            onRun?.();
            return true;
          },
        },
      ]),
    ];
  }, [placeholderText, onRun, engine, completion]);

  const themeExtensions = useMemo(
    () => [fleetdockEditorTheme(dark), syntaxTheme(dark)],
    [dark],
  );

  return (
    <div className="sql-editor">
      <CodeMirror
        value={value}
        height={minHeight}
        readOnly={readOnly}
        editable={!readOnly}
        extensions={extensions}
        theme={themeExtensions}
        basicSetup={{
          lineNumbers: false,
          highlightActiveLine: true,
          highlightActiveLineGutter: true,
          foldGutter: false,
          dropCursor: false,
          allowMultipleSelections: false,
          indentOnInput: true,
          bracketMatching: true,
          closeBrackets: false,
          autocompletion: !readOnly,
          rectangularSelection: false,
          crosshairCursor: false,
          highlightSelectionMatches: false,
          searchKeymap: false,
        }}
        onChange={(v) => onChange(v)}
        onCreateEditor={(view) => {
          viewRef.current = view;
        }}
      />
    </div>
  );
});
