"use client";

import { json, jsonParseLinter } from "@codemirror/lang-json";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { linter } from "@codemirror/lint";
import { Prec } from "@codemirror/state";
import { EditorView, keymap } from "@codemirror/view";
import { tags as t } from "@lezer/highlight";
import CodeMirror from "@uiw/react-codemirror";
import { useMemo } from "react";

import { fleetdockEditorTheme, useIsDark } from "@/components/sql-editor";

// The same colours as the JSON tree (JsonViewer), so editing looks like viewing.
const jsonColors = HighlightStyle.define([
  { tag: t.propertyName, color: "var(--syntax-function)" },
  { tag: t.string, color: "var(--syntax-string)" },
  { tag: t.number, color: "var(--syntax-number)" },
  { tag: [t.bool, t.null], color: "var(--syntax-keyword)" },
  { tag: [t.separator, t.squareBracket, t.brace, t.punctuation], color: "var(--syntax-operator)" },
]);

/**
 * JsonEditor edits a JSON value as plain text with syntax colours and a mark on
 * invalid JSON. Ctrl/Cmd+Enter applies, Escape cancels.
 */
export function JsonEditor({
  value,
  onChange,
  onApply,
  onCancel,
}: {
  value: string;
  onChange: (value: string) => void;
  /** Called with the editor's text. */
  onApply: (value: string) => void;
  onCancel: () => void;
}) {
  const dark = useIsDark();

  const extensions = useMemo(
    () => [
      json(),
      linter(jsonParseLinter(), { delay: 300 }),
      syntaxHighlighting(jsonColors),
      EditorView.lineWrapping,
      // Highest precedence: the default keymap binds Mod-Enter to insertBlankLine.
      Prec.highest(
        keymap.of([
          {
            key: "Mod-Enter",
            run(view) {
              onApply(view.state.doc.toString());
              return true;
            },
          },
          {
            key: "Escape",
            run() {
              onCancel();
              return true;
            },
          },
        ]),
      ),
    ],
    [onApply, onCancel],
  );
  const theme = useMemo(() => fleetdockEditorTheme(dark), [dark]);

  return (
    <div
      className="dbx-json-edit"
      // Keys typed here are the editor's, not the grid's or the workspace's.
      onKeyDown={(e) => e.stopPropagation()}
    >
      <CodeMirror
        value={value}
        height="100%"
        autoFocus
        extensions={extensions}
        theme={theme}
        basicSetup={{
          lineNumbers: false,
          foldGutter: false,
          highlightActiveLine: true,
          highlightActiveLineGutter: true,
          dropCursor: false,
          allowMultipleSelections: false,
          indentOnInput: true,
          bracketMatching: true,
          closeBrackets: true,
          autocompletion: false,
          rectangularSelection: false,
          crosshairCursor: false,
          highlightSelectionMatches: false,
          searchKeymap: false,
        }}
        onChange={onChange}
      />
    </div>
  );
}
