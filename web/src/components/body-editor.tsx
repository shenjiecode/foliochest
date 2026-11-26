import { useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  Annotation,
  Compartment,
  EditorState,
  Transaction,
} from "@codemirror/state";
import { EditorView, keymap, lineNumbers } from "@codemirror/view";
import {
  defaultKeymap,
  history,
  historyKeymap,
  isolateHistory,
} from "@codemirror/commands";
import {
  bracketMatching,
  defaultHighlightStyle,
  indentOnInput,
  indentUnit,
  syntaxHighlighting,
} from "@codemirror/language";
import { closeBrackets, closeBracketsKeymap } from "@codemirror/autocomplete";
import { json, jsonParseLinter } from "@codemirror/lang-json";
import { markdown } from "@codemirror/lang-markdown";
import { linter, lintGutter } from "@codemirror/lint";
import { getNonce } from "get-nonce";
import { Button } from "@/components/ui/button";
import { renderMarkdown } from "@/markdown";
import type { Entry } from "@/lib/types";

const external = Annotation.define<boolean>();
type Props = {
  value: string;
  format: Entry["format"];
  disabled?: boolean;
  onChange: (value: string) => void;
  id?: string;
  label?: string;
};

// Validate, then change whitespace only: never round numbers, reorder keys,
// collapse duplicate keys or rewrite string escapes via parse/stringify.
function prettyJSON(source: string): string {
  if (new TextEncoder().encode(source).length > 1024 * 1024)
    throw new Error("正文超过上限");
  JSON.parse(source);
  const tokens =
    source.match(/"(?:[^"\\]|\\.)*"|[^\s"{}\[\],:]+|[{}\[\],:]/g) || [];
  let depth = 0;
  const out: string[] = [];
  let length = 0;
  const append = (...parts: string[]) => {
    length += parts.reduce((sum, part) => sum + part.length, 0);
    if (length > 1024 * 1024) throw new Error("正文超过上限");
    out.push(...parts);
  };
  const newline = () => append("\n", "  ".repeat(Math.min(depth, 64)));
  tokens.forEach((token, i) => {
    if (token === "{" || token === "[") {
      append(token);
      depth++;
      if (tokens[i + 1] !== "}" && tokens[i + 1] !== "]") newline();
    } else if (token === "}" || token === "]") {
      depth--;
      if (tokens[i - 1] !== "{" && tokens[i - 1] !== "[") newline();
      append(token);
    } else if (token === ",") {
      append(token);
      newline();
    } else if (token === ":") append(": ");
    else append(token);
  });
  const result = out.join("");
  if (new TextEncoder().encode(result).length > 1024 * 1024)
    throw new Error("格式化后超过正文 1 MiB 上限，已保留原文。");
  return result;
}

export function BodyEditor(props: Props) {
  if (props.format === "text")
    return (
      <textarea
        id={props.id}
        className="source-editor body-source-editor"
        aria-label={props.label || "正文"}
        value={props.value}
        disabled={props.disabled}
        onChange={(e) => props.onChange(e.target.value)}
        spellCheck={false}
      />
    );
  return <StructuredEditor {...props} />;
}

function StructuredEditor({
  value,
  format,
  disabled = false,
  onChange,
  id,
  label = "正文",
}: Props) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const latest = useRef({ onChange, disabled });
  const language = useMemo(() => new Compartment(), []);
  const access = useMemo(() => new Compartment(), []);
  const [preview, setPreview] = useState(false);
  const [error, setError] = useState("");
  const html = useMemo(
    () => (preview && format === "markdown" ? renderMarkdown(value) : ""),
    [value, format, preview],
  );

  useLayoutEffect(() => {
    latest.current = { onChange, disabled };
  });
  useLayoutEffect(() => {
    const editor = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: value,
        extensions: [
          EditorView.cspNonce.of(getNonce() || ""),
          EditorView.contentAttributes.of({
            "aria-label": label,
            ...(id ? { id } : {}),
            spellcheck: "false",
          }),
          // Preserve exact input line separators, including CRLF, in the draft.
          EditorState.lineSeparator.of("\n"),
          lineNumbers(),
          history(),
          indentOnInput(),
          indentUnit.of("  "),
          bracketMatching(),
          closeBrackets(),
          syntaxHighlighting(defaultHighlightStyle),
          EditorView.lineWrapping,
          keymap.of([
            ...closeBracketsKeymap,
            ...defaultKeymap,
            ...historyKeymap,
          ]),
          language.of([]),
          access.of([]),
          EditorView.updateListener.of((update) => {
            if (
              update.docChanged &&
              !latest.current.disabled &&
              !update.transactions.some((t) => t.annotation(external))
            ) {
              latest.current.onChange(update.state.doc.toString());
            }
          }),
        ],
      }),
    });
    view.current = editor;
    return () => {
      view.current = null;
      editor.destroy();
    };
    // One instance per mounted document; formats and busy state reconfigure below.
  }, []);
  useLayoutEffect(() => {
    view.current?.dispatch({
      effects: language.reconfigure(
        format === "json"
          ? [json(), linter(jsonParseLinter()), lintGutter()]
          : markdown(),
      ),
    });
    setError("");
  }, [format, language]);
  useLayoutEffect(() => {
    view.current?.dispatch({
      effects: access.reconfigure([
        EditorState.readOnly.of(disabled),
        EditorView.editable.of(!disabled),
      ]),
    });
  }, [disabled, access]);
  useLayoutEffect(() => {
    const editor = view.current;
    if (editor && editor.state.doc.toString() !== value)
      editor.dispatch({
        changes: { from: 0, to: editor.state.doc.length, insert: value },
        annotations: [external.of(true), Transaction.addToHistory.of(false)],
      });
    setError("");
  }, [value]);

  const insert = (before: string, after = "", placeholder = "文字") => {
    const editor = view.current;
    if (!editor || disabled) return;
    const { from, to } = editor.state.selection.main;
    const selected = editor.state.sliceDoc(from, to) || placeholder;
    editor.dispatch({
      changes: { from, to, insert: before + selected + after },
      selection: {
        anchor: from + before.length,
        head: from + before.length + selected.length,
      },
      userEvent: "input",
    });
    editor.focus();
  };
  const formatJSON = () => {
    const editor = view.current;
    if (!editor || disabled) return;
    try {
      const result = prettyJSON(editor.state.doc.toString());
      editor.dispatch({
        changes: { from: 0, to: editor.state.doc.length, insert: result },
        annotations: isolateHistory.of("full"),
        userEvent: "input",
      });
      setError("");
      editor.focus();
    } catch (e) {
      setError(
        e instanceof SyntaxError
          ? "JSON 语法有误，请按红色标记修正后再格式化。原文未改动。"
          : "格式化后超过正文上限，已保留原文。",
      );
    }
  };
  return (
    <div className="body-editor">
      <div
        className="body-editor-toolbar"
        role="group"
        aria-label="正文编辑工具"
      >
        {format === "json" ? (
          <>
            <Button
              type="button"
              variant="ghost"
              disabled={disabled}
              onClick={formatJSON}
            >
              格式化 JSON
            </Button>
            <span className="field-note">
              语法错误会标红；悬停查看位置和原因
            </span>
          </>
        ) : (
          <>
            <Button
              type="button"
              variant="ghost"
              disabled={disabled}
              onClick={() => insert("**", "**")}
            >
              加粗
            </Button>
            <Button
              type="button"
              variant="ghost"
              disabled={disabled}
              onClick={() => insert("\n## ", "\n", "标题")}
            >
              标题
            </Button>
            <Button
              type="button"
              variant="ghost"
              disabled={disabled}
              onClick={() => insert("\n- ", "\n", "列表项")}
            >
              列表
            </Button>
            <Button
              type="button"
              variant="ghost"
              disabled={disabled}
              onClick={() => insert("[", "](https://)", "链接文字")}
            >
              链接
            </Button>
            <Button
              type="button"
              variant="ghost"
              disabled={disabled}
              onClick={() => insert("\n```\n", "\n```\n", "代码")}
            >
              代码块
            </Button>
            <Button
              type="button"
              variant={preview ? "secondary" : "ghost"}
              disabled={disabled}
              aria-pressed={preview}
              onClick={() => setPreview((p) => !p)}
            >
              {preview ? "收起预览" : "预览"}
            </Button>
          </>
        )}
      </div>
      <div ref={host} className="code-editor-host" />
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {format === "markdown" && preview && (
        <section className="body-editor-preview" aria-label="Markdown 预览">
          <h4>预览</h4>
          <article
            className="article"
            dangerouslySetInnerHTML={{ __html: html }}
          />
        </section>
      )}
    </div>
  );
}
