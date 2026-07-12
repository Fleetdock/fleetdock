"use client";

import { indentWithTab } from "@codemirror/commands";
import { sql } from "@codemirror/lang-sql";
import { type Extension } from "@codemirror/state";
import { EditorView, keymap, lineNumbers, placeholder } from "@codemirror/view";
import CodeMirror from "@uiw/react-codemirror";
import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";

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
};

export const SqlEditor = forwardRef<SqlEditorHandle, SqlEditorProps>(function SqlEditor(
  { value, onChange, onRun, placeholder: placeholderText = "SELECT * FROM ...", minHeight = "200px" },
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
      sql(),
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
  }, [placeholderText, onRun]);

  const themeExtensions = useMemo(
    () => [fleetdockEditorTheme(dark), syntaxTheme(dark)],
    [dark],
  );

  return (
    <div className="sql-editor">
      <CodeMirror
        value={value}
        height={minHeight}
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
          autocompletion: false,
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
