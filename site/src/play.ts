// The playground: a CodeMirror editor, Evie running in a worker, an output
// pane that works like a terminal, examples and shareable links.
import "./common";
import { defaultKeymap, history as editHistory, historyKeymap, indentWithTab } from "@codemirror/commands";
import { bracketMatching, HighlightStyle, indentUnit, StreamLanguage, syntaxHighlighting } from "@codemirror/language";
import { EditorState, Prec } from "@codemirror/state";
import { drawSelection, EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import { startState, token } from "./evie-syntax.js";
import examples from "./generated/examples.json";

type Example = { id: string; title: string; code: string };
const list = examples as Example[];

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
const runButton = $<HTMLButtonElement>("run");
const stopButton = $<HTMLButtonElement>("stop");
const shareButton = $<HTMLButtonElement>("share");
const clearButton = $<HTMLButtonElement>("clear");
const status = $<HTMLSpanElement>("status");
const output = $<HTMLDivElement>("panel-output");
const pickerRoot = $<HTMLDivElement>("example");

// on a Mac the editor's Mod-Enter is ⌘ Enter, so the labels say so
const isMac = /mac|iphone|ipad/i.test((navigator as any).userAgentData?.platform ?? navigator.platform ?? "");
if (isMac) {
  for (const kbd of document.querySelectorAll("kbd[data-mod]")) kbd.textContent = "⌘ ↵";
}

const storageKey = "evie-playground";
const maxOutput = 200_000; // characters kept on screen; a runaway print loop stops growing the page

// ---------------------------------------------------------------- editor

const evie = StreamLanguage.define<ReturnType<typeof startState>>({
  name: "evie",
  startState: () => startState(),
  copyState: (s) => ({ ...s, braces: [...s.braces] }),
  token: (stream, state) => token(stream, state),
  languageData: { commentTokens: { line: "//", block: { open: "/*", close: "*/" } } },
  tokenTable: {
    function: tags.function(tags.variableName),
    type: tags.typeName,
    builtin: tags.standard(tags.variableName),
    property: tags.propertyName,
  },
});

const highlighting = HighlightStyle.define([
  { tag: tags.keyword, class: "t-keyword" },
  { tag: tags.atom, class: "t-atom" },
  { tag: tags.string, class: "t-string" },
  { tag: tags.number, class: "t-number" },
  { tag: tags.comment, class: "t-comment" },
  { tag: tags.function(tags.variableName), class: "t-function" },
  { tag: tags.typeName, class: "t-type" },
  { tag: tags.standard(tags.variableName), class: "t-builtin" },
  { tag: tags.propertyName, class: "t-property" },
  { tag: tags.operator, class: "t-operator" },
  { tag: tags.punctuation, class: "t-punctuation" },
]);

const view = new EditorView({
  parent: $("editor"),
  state: EditorState.create({
    doc: "",
    extensions: [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightActiveLine(),
      drawSelection(),
      editHistory(),
      bracketMatching(),
      indentUnit.of("    "),
      evie,
      syntaxHighlighting(highlighting),
      Prec.highest(keymap.of([{ key: "Mod-Enter", run: () => (run(), true) }, { key: "Mod-s", run: () => (share(), true) }])),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      EditorView.updateListener.of((u) => {
        if (!u.docChanged) return;
        save();
        // an example that has been edited is the user's own code
        if (picker.value && u.state.doc.toString() !== list.find((e) => e.id === picker.value)?.code) picker.value = "";
      }),
      EditorView.contentAttributes.of({ "aria-label": "Evie code" }),
    ],
  }),
});

function setCode(code: string) {
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: code } });
}

function save() {
  try {
    localStorage.setItem(storageKey, JSON.stringify({ code: view.state.doc.toString() }));
  } catch {
    // storage may be unavailable; the code then just isn't remembered
  }
}

// ---------------------------------------------------------------- examples

// The picker is a button that opens a listbox of the examples. Its value is
// the id of the example in the editor, or "" for code of the user's own,
// which the button then calls "Your code".
const pickerButton = pickerRoot.querySelector("button")!;
const pickerLabel = $<HTMLSpanElement>("example-current");
const menu = pickerRoot.querySelector("ul")!;

const heading = document.createElement("li");
heading.className = "picker__heading";
heading.setAttribute("role", "presentation");
heading.textContent = "Examples";
menu.append(heading);
const items = list.map((ex, i) => {
  const li = document.createElement("li");
  li.id = `example-${ex.id}`;
  li.setAttribute("role", "option");
  li.textContent = ex.title;
  li.addEventListener("click", () => choose(i));
  li.addEventListener("mousemove", () => highlight(i));
  menu.append(li);
  return li;
});

let selected = "";
let active = -1;
const picker = {
  get value() {
    return selected;
  },
  set value(id: string) {
    const ex = list.find((e) => e.id === id);
    selected = ex ? ex.id : "";
    pickerLabel.textContent = ex ? ex.title : "Your code";
    items.forEach((li, i) => li.setAttribute("aria-selected", String(list[i].id === selected)));
  },
};

function highlight(i: number) {
  active = Math.max(0, Math.min(items.length - 1, i));
  items.forEach((li, j) => li.classList.toggle("is-active", j === active));
  menu.setAttribute("aria-activedescendant", items[active].id);
  items[active].scrollIntoView({ block: "nearest" });
}

function openMenu() {
  menu.hidden = false;
  pickerButton.setAttribute("aria-expanded", "true");
  highlight(Math.max(0, list.findIndex((e) => e.id === selected)));
  menu.focus();
}

function closeMenu(focusButton = true) {
  if (menu.hidden) return;
  menu.hidden = true;
  pickerButton.setAttribute("aria-expanded", "false");
  if (focusButton) pickerButton.focus();
}

function choose(i: number) {
  const ex = list[i];
  closeMenu(false);
  picker.value = ex.id;
  setCode(ex.code);
  save();
  history.replaceState(null, "", `#example=${ex.id}`);
  view.focus();
}

pickerButton.addEventListener("click", () => (menu.hidden ? openMenu() : closeMenu()));
pickerButton.addEventListener("keydown", (e) => {
  if (["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) {
    e.preventDefault();
    openMenu();
  }
});
menu.addEventListener("keydown", (e) => {
  switch (e.key) {
    case "ArrowDown":
      highlight(active + 1);
      break;
    case "ArrowUp":
      highlight(active - 1);
      break;
    case "Home":
      highlight(0);
      break;
    case "End":
      highlight(items.length - 1);
      break;
    case "Enter":
    case " ":
      choose(active);
      break;
    case "Escape":
      closeMenu();
      break;
    case "Tab":
      closeMenu(false);
      return; // let the focus move on
    default:
      // a letter jumps to the next example whose title starts with it
      if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
        const k = e.key.toLowerCase();
        for (let n = 1; n <= items.length; n++) {
          const j = (active + n) % items.length;
          if (list[j].title.toLowerCase().startsWith(k)) {
            highlight(j);
            break;
          }
        }
      }
      return;
  }
  e.preventDefault();
});
// a click or the focus anywhere else closes the menu
document.addEventListener("pointerdown", (e) => {
  if (!pickerRoot.contains(e.target as Node)) closeMenu(false);
});
menu.addEventListener("focusout", (e) => {
  if (!pickerRoot.contains(e.relatedTarget as Node | null)) closeMenu(false);
});

// ---------------------------------------------------------------- sharing

const bytesToBase64url = (bytes: Uint8Array) => btoa(String.fromCharCode(...bytes)).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
const base64urlToBytes = (s: string) => Uint8Array.from(atob(s.replaceAll("-", "+").replaceAll("_", "/")), (c) => c.charCodeAt(0));

async function compress(text: string): Promise<string> {
  const stream = new Blob([text]).stream().pipeThrough(new CompressionStream("deflate-raw"));
  return bytesToBase64url(new Uint8Array(await new Response(stream).arrayBuffer()));
}

async function decompress(data: string): Promise<string> {
  const stream = new Blob([base64urlToBytes(data)]).stream().pipeThrough(new DecompressionStream("deflate-raw"));
  return new Response(stream).text();
}

async function share() {
  const code = view.state.doc.toString();
  const hash = "CompressionStream" in window ? `#z=${await compress(code)}` : `#code=${bytesToBase64url(new TextEncoder().encode(code))}`;
  history.replaceState(null, "", hash);
  picker.value = "";
  try {
    await navigator.clipboard.writeText(location.href);
    setStatus("Link copied", "ok");
  } catch {
    setStatus("The link is in the address bar", "ok");
  }
}
shareButton.addEventListener("click", share);

async function initialCode(): Promise<string> {
  const params = new URLSearchParams(location.hash.slice(1));
  try {
    if (params.has("z") || params.has("code")) picker.value = ""; // shared code is not one of the examples
    if (params.has("z")) return await decompress(params.get("z")!);
    if (params.has("code")) return new TextDecoder().decode(base64urlToBytes(params.get("code")!));
  } catch {
    setStatus("That link's code could not be read", "error");
  }
  const ex = list.find((e) => e.id === params.get("example"));
  if (ex) {
    picker.value = ex.id;
    return ex.code;
  }
  try {
    const saved = JSON.parse(localStorage.getItem(storageKey) ?? "null");
    if (saved?.code) {
      // the picker names the example only while the code is still that example
      picker.value = list.find((e) => e.code === saved.code)?.id ?? "";
      return saved.code;
    }
  } catch {
    // nothing saved, or storage unavailable
  }
  picker.value = list[0]?.id ?? "";
  return list[0]?.code ?? "";
}

// ---------------------------------------------------------------- running

let worker: Worker | null = null;
let ready = false;
let running = false;
let written = 0;
let outNode: HTMLSpanElement | null = null;

function setStatus(text: string, kind: "" | "ok" | "error" = "") {
  status.textContent = text;
  status.className = `status${kind ? ` is-${kind}` : ""}`;
}

function startWorker() {
  ready = false;
  runButton.disabled = true;
  worker = new Worker(`${import.meta.env.BASE_URL}evie-worker.js`);
  worker.onmessage = (event) => {
    const msg = event.data;
    switch (msg.type) {
      case "ready":
        ready = true;
        runButton.disabled = false;
        if (!running) setStatus("Ready");
        break;
      case "out":
        write(msg.text);
        break;
      case "read":
        ask();
        break;
      case "done":
        finish(msg);
        break;
      case "failed":
        setStatus("Evie could not load in this browser", "error");
        break;
    }
  };
  worker.onerror = () => setStatus("Evie stopped unexpectedly", "error");
}

// out is where a run's printed text and typed lines go, made on first use
function out(): HTMLSpanElement {
  if (!outNode) {
    output.replaceChildren();
    outNode = document.createElement("span");
    outNode.className = "out";
    output.append(outNode);
  }
  return outNode;
}

function write(text: string) {
  if (written >= maxOutput) return;
  const room = maxOutput - written;
  out().append(text.length > room ? text.slice(0, room) + "\n… output cut off here" : text);
  written += text.length;
  output.scrollTop = output.scrollHeight;
}

function line(className: string, text: string) {
  out();
  const el = document.createElement("span");
  el.className = className;
  el.textContent = text;
  output.append(el);
  output.scrollTop = output.scrollHeight;
}

// ---------------------------------------------------------------- input

// field is where the user types the line the program is waiting for, placed
// right after what it printed, such as the message of io.prompt
let field: HTMLInputElement | null = null;

function ask() {
  closeField();
  const f = document.createElement("input");
  f.className = "term-input";
  f.setAttribute("aria-label", "A line for the program to read");
  f.autocomplete = "off";
  f.spellcheck = false;
  f.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      const text = f.value;
      closeField();
      const typed = document.createElement("span");
      typed.className = "typed";
      typed.textContent = text;
      out().append(typed, "\n");
      written += text.length + 1;
      worker?.postMessage({ type: "input", line: text });
      setStatus("Running…");
    } else if (e.key.toLowerCase() === "d" && (e.ctrlKey || e.metaKey) && f.value === "") {
      // Ctrl+D on an empty line ends the input, as in a terminal
      e.preventDefault();
      closeField();
      worker?.postMessage({ type: "eof" });
      setStatus("Running…");
    }
  });
  field = f;
  out().append(f);
  f.focus({ preventScroll: true });
  output.scrollTop = output.scrollHeight;
  setStatus("Waiting for input");
}

function closeField() {
  field?.remove();
  field = null;
}

// a click anywhere in the output, other than to select text, goes to the field
output.addEventListener("click", () => {
  if (field && !getSelection()?.toString()) field.focus();
});

function formatMs(ms: number) {
  return ms < 1 ? `${(ms * 1000).toFixed(0)} µs` : ms < 1000 ? `${ms.toFixed(ms < 10 ? 2 : 1)} ms` : `${(ms / 1000).toFixed(2)} s`;
}

function finish(msg: { ms: number; result?: string; error?: string }) {
  running = false;
  closeField();
  runButton.hidden = false;
  stopButton.hidden = true;
  if (msg.result !== undefined) line("result", msg.result);
  if (msg.error) line("error", msg.error.trimEnd());
  if (!msg.error && msg.result === undefined && written === 0) line("meta", "The program printed nothing and returned nothing.");
  line("meta", `Ran in ${formatMs(msg.ms)}`);
  setStatus(msg.error ? "Finished with an error" : `Finished in ${formatMs(msg.ms)}`, msg.error ? "error" : "ok");
}

function run() {
  if (!ready || running || !worker) return;
  closeField();
  running = true;
  written = 0;
  outNode = null;
  output.replaceChildren();
  runButton.hidden = true;
  stopButton.hidden = false;
  setStatus("Running…");
  worker.postMessage({ type: "run", code: view.state.doc.toString() });
}

function stop() {
  if (!running) return;
  worker?.terminate();
  running = false;
  closeField();
  runButton.hidden = false;
  stopButton.hidden = true;
  line("meta", "Stopped.");
  setStatus("Stopped; restarting Evie…");
  startWorker();
}

runButton.addEventListener("click", run);
stopButton.addEventListener("click", stop);
clearButton.addEventListener("click", () => {
  // a program waiting for a line keeps waiting, with the field on a clean pane
  const waiting = field;
  output.replaceChildren();
  outNode = null;
  written = 0;
  if (waiting) {
    out().append(waiting);
    waiting.focus();
  }
});

// ---------------------------------------------------------------- start

// a link pasted into an open playground changes only the hash
addEventListener("hashchange", () => {
  initialCode().then(setCode);
});

startWorker();
initialCode().then(setCode);
