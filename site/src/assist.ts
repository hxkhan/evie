// The editor's help: errors as you type, completion and hover, answered by
// Evie's own analysis (the one the VS Code extension uses) in a worker of its
// own, so a running or stopped program never holds it up. The worker loads the
// same evie.wasm as the one that runs programs, which the browser caches.
import { autocompletion, type Completion, type CompletionContext, type CompletionResult } from "@codemirror/autocomplete";
import { type Diagnostic, linter, lintGutter } from "@codemirror/lint";
import type { Extension, Text } from "@codemirror/state";
import { EditorView, hoverTooltip } from "@codemirror/view";

// the analysis's answers, in the Language Server Protocol's terms
type Position = { line: number; character: number };
type Range = { start: Position; end: Position };
type Fix = { title: string; edit: { range: Range; newText: string } };
type LspDiagnostic = { range: Range; severity: number; message: string; tags?: number[]; data?: { fixes: Fix[] } };
type LspCompletion = { label: string; kind: number; detail?: string; documentation?: string };
type LspHover = { contents: string; range: Range };

// ---------------------------------------------------------------- worker

let worker: Worker | null = null;
let ready: Promise<void> | null = null;
let nextId = 0;
const waiting = new Map<number, (json: string) => void>();

function start(url: string) {
  worker = new Worker(url);
  ready = new Promise((resolve, reject) => {
    worker!.onmessage = (event) => {
      const msg = event.data;
      if (msg.type === "ready") resolve();
      else if (msg.type === "failed") reject(new Error(msg.error));
      else if (msg.type === "answer") {
        waiting.get(msg.id)?.(msg.json);
        waiting.delete(msg.id);
      }
    };
  });
  // the help is a convenience: if it cannot load, the editor works without it
  ready.catch(() => (worker = null));
  worker.onerror = () => {
    worker = null;
    for (const answer of waiting.values()) answer("null");
    waiting.clear();
  };
}

async function ask<T>(type: "check" | "complete" | "hover", ...args: unknown[]): Promise<T | null> {
  try {
    await ready;
  } catch {
    return null;
  }
  if (!worker) return null;
  const id = nextId++;
  const json = await new Promise<string>((resolve) => {
    waiting.set(id, resolve);
    worker!.postMessage({ type, id, args });
  });
  return JSON.parse(json) as T | null;
}

// ---------------------------------------------------------------- positions

// CodeMirror and the protocol both count UTF-16 code units, so a position is
// the line's start plus the character
function offset(doc: Text, p: Position): number {
  if (p.line >= doc.lines) return doc.length;
  const line = doc.line(p.line + 1);
  return Math.min(line.from + p.character, line.to);
}

function position(doc: Text, pos: number): Position {
  const line = doc.lineAt(pos);
  return { line: line.number - 1, character: pos - line.from };
}

// ---------------------------------------------------------------- errors

const errors = linter(
  async (view) => {
    const doc = view.state.doc;
    const found = (await ask<LspDiagnostic[]>("check", doc.toString())) ?? [];
    return found.map((d): Diagnostic => {
      const from = offset(doc, d.range.start);
      const to = Math.max(from, offset(doc, d.range.end));
      return {
        from,
        to,
        severity: d.severity === 1 ? "error" : "hint",
        message: d.message,
        markClass: d.tags?.includes(1) ? "cm-unused" : undefined,
        actions: (d.data?.fixes ?? []).map((fix) => ({
          name: fix.title,
          apply(view: EditorView) {
            const doc = view.state.doc;
            view.dispatch({ changes: { from: offset(doc, fix.edit.range.start), to: offset(doc, fix.edit.range.end), insert: fix.edit.newText } });
          },
        })),
      };
    });
  },
  { delay: 250 },
);

// ---------------------------------------------------------------- completion

const kinds: Record<number, string> = { 2: "method", 3: "function", 5: "property", 6: "variable", 7: "class", 9: "namespace", 14: "keyword", 21: "constant" };

async function complete(context: CompletionContext): Promise<CompletionResult | null> {
  const word = context.matchBefore(/[A-Za-z_][A-Za-z0-9_]*/);
  const afterDot = /\.\s*$/.test(context.state.sliceDoc(Math.max(0, (word?.from ?? context.pos) - 8), word?.from ?? context.pos));
  if (!word && !afterDot && !context.explicit) return null;
  const doc = context.state.doc;
  const at = position(doc, context.pos);
  const items = await ask<LspCompletion[]>("complete", doc.toString(), at.line, at.character);
  if (!items?.length || context.aborted) return null;
  return {
    from: word?.from ?? context.pos,
    validFor: /^[A-Za-z0-9_]*$/,
    options: items.map(
      (c): Completion => ({
        label: c.label,
        type: kinds[c.kind] ?? "variable",
        detail: c.detail,
        info: c.documentation ? () => markdown(c.documentation!) : undefined,
      }),
    ),
  };
}

// ---------------------------------------------------------------- hover

const hover = hoverTooltip(async (view, pos) => {
  const doc = view.state.doc;
  const at = position(doc, pos);
  const h = await ask<LspHover>("hover", doc.toString(), at.line, at.character);
  if (!h) return null;
  return {
    pos: offset(doc, h.range.start),
    end: offset(doc, h.range.end),
    above: true,
    create: () => ({ dom: markdown(h.contents, "cm-evie-hover") }),
  };
});

// markdown renders the little Markdown the analysis writes: ``` blocks,
// paragraphs, and `code`, **bold** and *italic* inside them
function markdown(text: string, className = "cm-evie-doc"): HTMLElement {
  const root = document.createElement("div");
  root.className = className;
  const parts = text.split(/```[a-z]*\n([\s\S]*?)\n?```/);
  parts.forEach((part, i) => {
    if (i % 2 === 1) {
      const pre = document.createElement("pre");
      pre.textContent = part;
      root.append(pre);
      return;
    }
    for (const para of part.split(/\n{2,}/)) {
      if (!para.trim()) continue;
      const p = document.createElement("p");
      inline(p, para.trim());
      root.append(p);
    }
  });
  return root;
}

// inline appends text to el with its `code`, **bold** and *italic* parts,
// which may hold code themselves
function inline(el: HTMLElement, text: string) {
  for (const [, code, bold, italic, plain] of text.matchAll(/`([^`]+)`|\*\*([^*]+)\*\*|\*([^*]+)\*|([^`*]+|[`*])/g)) {
    if (code !== undefined) {
      const c = document.createElement("code");
      c.textContent = code;
      el.append(c);
    } else if (bold !== undefined || italic !== undefined) {
      const e = document.createElement(bold !== undefined ? "strong" : "em");
      inline(e, bold ?? italic);
      el.append(e);
    } else el.append(plain);
  }
}

// assist is the help for an editor, with url the worker script that loads Evie
export function assist(url: string): Extension[] {
  start(url);
  return [errors, lintGutter(), autocompletion({ override: [complete], icons: false }), hover];
}
