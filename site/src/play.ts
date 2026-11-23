// The playground: a CodeMirror editor, Evie running in a worker, the output,
// standard input, examples and shareable links.
import "./common";
import { defaultKeymap, history as editHistory, historyKeymap, indentWithTab } from "@codemirror/commands";
import { bracketMatching, HighlightStyle, indentUnit, StreamLanguage, syntaxHighlighting } from "@codemirror/language";
import { EditorState, Prec } from "@codemirror/state";
import { drawSelection, EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import { startState, token } from "./evie-syntax.js";
import examples from "./generated/examples.json";

type Example = { id: string; title: string; code: string; stdin?: string };
const list = examples as Example[];

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
const runButton = $<HTMLButtonElement>("run");
const stopButton = $<HTMLButtonElement>("stop");
const shareButton = $<HTMLButtonElement>("share");
const clearButton = $<HTMLButtonElement>("clear");
const status = $<HTMLSpanElement>("status");
const output = $<HTMLDivElement>("panel-output");
const input = $<HTMLDivElement>("panel-input");
const stdin = $<HTMLTextAreaElement>("stdin");
const picker = $<HTMLSelectElement>("example");
const tabOutput = $<HTMLButtonElement>("tab-output");
const tabInput = $<HTMLButtonElement>("tab-input");

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
        if (u.docChanged) save();
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
    localStorage.setItem(storageKey, JSON.stringify({ code: view.state.doc.toString(), stdin: stdin.value }));
  } catch {
    // storage may be unavailable; the code then just isn't remembered
  }
}
stdin.addEventListener("input", save);

// ---------------------------------------------------------------- examples

for (const ex of list) {
  const option = document.createElement("option");
  option.value = ex.id;
  option.textContent = ex.title;
  picker.append(option);
}
const custom = document.createElement("option");
custom.value = "";
custom.textContent = "Your code";
picker.prepend(custom);

picker.addEventListener("change", () => {
  const ex = list.find((e) => e.id === picker.value);
  if (ex) {
    setCode(ex.code);
    stdin.value = ex.stdin ?? "";
    save();
    history.replaceState(null, "", `#example=${ex.id}`);
    view.focus();
  }
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
    stdin.value = ex.stdin ?? "";
    return ex.code;
  }
  try {
    const saved = JSON.parse(localStorage.getItem(storageKey) ?? "null");
    if (saved?.code) {
      stdin.value = saved.stdin ?? "";
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

function write(text: string) {
  if (written >= maxOutput) return;
  if (!outNode) {
    output.replaceChildren();
    outNode = document.createElement("span");
    outNode.className = "out";
    output.append(outNode);
  }
  const room = maxOutput - written;
  outNode.append(text.length > room ? text.slice(0, room) + "\n… output cut off here" : text);
  written += text.length;
  output.scrollTop = output.scrollHeight;
}

function line(className: string, text: string) {
  if (!outNode) {
    output.replaceChildren();
    outNode = document.createElement("span");
    outNode.className = "out";
    output.append(outNode);
  }
  const el = document.createElement("span");
  el.className = className;
  el.textContent = text;
  output.append(el);
  output.scrollTop = output.scrollHeight;
}

function formatMs(ms: number) {
  return ms < 1 ? `${(ms * 1000).toFixed(0)} µs` : ms < 1000 ? `${ms.toFixed(ms < 10 ? 2 : 1)} ms` : `${(ms / 1000).toFixed(2)} s`;
}

function finish(msg: { ms: number; result?: string; error?: string }) {
  running = false;
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
  showTab("output");
  running = true;
  written = 0;
  outNode = null;
  output.replaceChildren();
  runButton.hidden = true;
  stopButton.hidden = false;
  setStatus("Running…");
  worker.postMessage({ type: "run", code: view.state.doc.toString(), stdin: stdin.value });
}

function stop() {
  if (!running) return;
  worker?.terminate();
  running = false;
  runButton.hidden = false;
  stopButton.hidden = true;
  line("meta", "Stopped.");
  setStatus("Stopped; restarting Evie…");
  startWorker();
}

runButton.addEventListener("click", run);
stopButton.addEventListener("click", stop);
clearButton.addEventListener("click", () => {
  output.replaceChildren();
  outNode = null;
  written = 0;
});

// ---------------------------------------------------------------- tabs

function showTab(which: "output" | "input") {
  const isOutput = which === "output";
  tabOutput.setAttribute("aria-selected", String(isOutput));
  tabInput.setAttribute("aria-selected", String(!isOutput));
  output.hidden = !isOutput;
  input.hidden = isOutput;
  clearButton.hidden = !isOutput;
}
tabOutput.addEventListener("click", () => showTab("output"));
tabInput.addEventListener("click", () => showTab("input"));

// ---------------------------------------------------------------- start

// a link pasted into an open playground changes only the hash
addEventListener("hashchange", () => {
  initialCode().then(setCode);
});

startWorker();
initialCode().then((code) => {
  setCode(code);
  if (/io\.(readln|prompt)\(/.test(code) && !stdin.value) {
    stdin.value = "Ada\n";
  }
});
