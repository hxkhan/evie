// End-to-end test of the language server: starts out/server.js over stdio on a
// workspace of Evie files and talks to it the way VS Code does.
//
//	npm run build && node scripts/test-server.mjs
import { spawn } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const rpc = require("vscode-jsonrpc/node");
const { URI } = require("vscode-uri");

const here = dirname(fileURLToPath(import.meta.url));
const server = join(here, "..", "out", "server.js");

const files = {
  "main.ev": `package main imports("io", "./util", "./lib")

const limit := 3

class Animal(name) {
    fn speak() => this.name
}

class Dog(name) : Animal {
    fn bark(times) {
        io.println(this.speak())
        return util.greet(times) + lib.twice(limit)
    }
}

fn main() {
    d := Dog("rex")
    return d.bark(limit)
}
`,
  "util.ev": 'package util\nfn greet(n) => "hi " + n\n',
  "lib/a.ev": "package lib\nfn twice(n) => half(n) * 4\n",
  "lib/b.ev": "package lib\nfn half(n) => n / 2\n",
};

const root = mkdtempSync(join(tmpdir(), "evie-lsp-"));
for (const [name, text] of Object.entries(files)) {
  mkdirSync(dirname(join(root, name)), { recursive: true });
  writeFileSync(join(root, name), text);
}
const uri = (name) => URI.file(join(root, name)).toString();

const proc = spawn(process.execPath, [server, "--stdio"], { stdio: ["pipe", "pipe", "inherit"] });
proc.stdin.on("error", () => {}); // the server may be gone by the time exit is sent
const conn = rpc.createMessageConnection(new rpc.StreamMessageReader(proc.stdout), new rpc.StreamMessageWriter(proc.stdin));

const diagnostics = new Map();
const waiters = [];
conn.onNotification("textDocument/publishDiagnostics", (p) => {
  diagnostics.set(p.uri, p.diagnostics);
  for (const w of [...waiters]) w();
});
conn.listen();

// waitFor resolves once check() holds for the diagnostics, or fails after a while
function waitFor(what, check, ms = 10000) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`timed out waiting for ${what}: ${JSON.stringify([...diagnostics])}`)), ms);
    const w = () => {
      if (check()) {
        clearTimeout(timer);
        waiters.splice(waiters.indexOf(w), 1);
        resolve();
      }
    };
    waiters.push(w);
    w();
  });
}

let failed = 0;
function ok(name, cond, detail) {
  if (!cond) failed++;
  console.log(`${cond ? "ok  " : "FAIL"} ${name}${cond ? "" : " " + JSON.stringify(detail)}`);
}

// pos is the position of a word on a line of a text
function pos(text, line, word) {
  return { line, character: text.split("\n")[line].indexOf(word) };
}

const main = files["main.ev"];
try {
  await conn.sendRequest("initialize", {
    processId: process.pid,
    rootUri: URI.file(root).toString(),
    workspaceFolders: [{ uri: URI.file(root).toString(), name: "test" }],
    capabilities: {},
  });
  conn.sendNotification("initialized", {});

  conn.sendNotification("textDocument/didOpen", { textDocument: { uri: uri("main.ev"), languageId: "evie", version: 1, text: main } });
  await waitFor("main.ev's diagnostics", () => diagnostics.has(uri("main.ev")));
  ok("correct code has no errors", diagnostics.get(uri("main.ev")).length === 0, diagnostics.get(uri("main.ev")));
  ok("the imported files are covered too", diagnostics.has(uri("lib/b.ev")), [...diagnostics.keys()]);

  const def = await conn.sendRequest("textDocument/definition", { textDocument: { uri: uri("main.ev") }, position: pos(main, 11, "greet") });
  ok("go to a definition in another file", def && def.uri === uri("util.ev") && def.range.start.line === 1, def);

  const inherited = await conn.sendRequest("textDocument/definition", { textDocument: { uri: uri("main.ev") }, position: pos(main, 10, "speak") });
  ok("go to an inherited method", inherited && inherited.range.start.line === 5, inherited);

  const refs = await conn.sendRequest("textDocument/references", {
    textDocument: { uri: uri("main.ev") },
    position: pos(main, 2, "limit"),
    context: { includeDeclaration: true },
  });
  ok("find every reference", refs.length === 3, refs);

  const hover = await conn.sendRequest("textDocument/hover", { textDocument: { uri: uri("main.ev") }, position: pos(main, 10, "println") });
  ok("hover shows what a standard function does", hover?.contents?.value?.includes("Writes the values"), hover);

  const symbols = await conn.sendRequest("textDocument/documentSymbol", { textDocument: { uri: uri("main.ev") } });
  ok("outline", symbols.map((s) => s.name).join(" ") === "limit Animal Dog main", symbols.map((s) => s.name));

  const folds = await conn.sendRequest("textDocument/foldingRange", { textDocument: { uri: uri("main.ev") } });
  ok("fold the blocks", folds.some((f) => f.startLine === 8 && f.endLine === 12), folds);

  const marks = await conn.sendRequest("textDocument/documentHighlight", { textDocument: { uri: uri("main.ev") }, position: pos(main, 2, "limit") });
  ok("highlight the occurrences", marks.length === 3 && marks.filter((m) => m.kind === 3).length === 1, marks);

  const prepared = await conn.sendRequest("textDocument/prepareRename", { textDocument: { uri: uri("main.ev") }, position: pos(main, 2, "limit") });
  ok("prepare a rename", prepared?.placeholder === "limit", prepared);
  const renamed = await conn.sendRequest("textDocument/rename", {
    textDocument: { uri: uri("main.ev") },
    position: pos(main, 11, "greet"),
    newName: "welcome",
  });
  const files = Object.keys(renamed?.changes ?? {}).map((u) => u.split("/").pop()).sort().join(" ");
  ok("rename a function across files", files === "main.ev util.ev", renamed);
  const refused = await conn
    .sendRequest("textDocument/rename", { textDocument: { uri: uri("main.ev") }, position: pos(main, 2, "limit"), newName: "while" })
    .then(() => null, (err) => err.message);
  ok("refuse a rename that cannot work, saying why", refused?.includes("keyword"), refused);

  // completion while the line being typed does not parse
  const typing = main.replace("    return d.bark(limit)\n", "    io.\n    return d.bark(limit)\n");
  conn.sendNotification("textDocument/didChange", {
    textDocument: { uri: uri("main.ev"), version: 2 },
    contentChanges: [{ text: typing }],
  });
  const items = await conn.sendRequest("textDocument/completion", { textDocument: { uri: uri("main.ev") }, position: { line: 17, character: 7 } });
  const labels = (Array.isArray(items) ? items : items.items).map((i) => i.label);
  ok("complete the members of a package", labels.includes("println") && labels.includes("readln"), labels);

  // signature help while the call is being typed
  const calling = main.replace("    return d.bark(limit)\n", "    util.greet(\n    return d.bark(limit)\n");
  conn.sendNotification("textDocument/didChange", { textDocument: { uri: uri("main.ev"), version: 3 }, contentChanges: [{ text: calling }] });
  const help = await conn.sendRequest("textDocument/signatureHelp", { textDocument: { uri: uri("main.ev") }, position: { line: 17, character: 15 } });
  ok("show the signature of the call being typed", help?.signatures?.[0]?.label === "util.greet(n)" && help.activeParameter === 0, help);

  await waitFor("the error in the edited file", () => diagnostics.get(uri("main.ev"))?.length > 0);
  ok("an edit that breaks the file is reported", diagnostics.get(uri("main.ev")).length > 0);

  // an unsaved change to an imported file counts, not what is on disk
  conn.sendNotification("textDocument/didOpen", {
    textDocument: { uri: uri("util.ev"), languageId: "evie", version: 1, text: "package util\nfn greet(n) => missingName\n" },
  });
  await waitFor("util.ev's unsaved error", () => (diagnostics.get(uri("util.ev")) ?? []).some((d) => d.message.includes("missingName")));
  ok("an unsaved buffer of an imported file is what is checked", true);

  const found = await conn.sendRequest("workspace/symbol", { query: "twi" });
  ok("search the workspace's symbols", found.length === 1 && found[0].name === "twice" && found[0].location.uri === uri("lib/a.ev"), found);

  const links = await conn.sendRequest("textDocument/documentLink", { textDocument: { uri: uri("main.ev") } });
  ok("import paths link to what they import", links.map((l) => l.target).join(" ") === `${uri("util.ev")} ${uri("lib/a.ev")}`, links);

  const lenses = await conn.sendRequest("textDocument/codeLens", { textDocument: { uri: uri("util.ev") } });
  ok("no Run link on a package other than main", lenses.length === 0, lenses);
  const scriptLens = await conn.sendRequest("textDocument/codeLens", { textDocument: { uri: uri("lib/b.ev") } });
  ok("no Run link on a package folder", scriptLens.length === 0, scriptLens);

  // a hint for a name nothing uses, and a quick fix for a misspelt one
  conn.sendNotification("textDocument/didOpen", { textDocument: { uri: uri("hint.ev"), languageId: "evie", version: 1, text: "x := 1\n" } });
  await waitFor("hint.ev's hint", () => diagnostics.get(uri("hint.ev"))?.length > 0);
  const hint = diagnostics.get(uri("hint.ev"))[0];
  ok("an unused name gets a faded hint", hint.severity === 4 && hint.tags?.includes(1), hint);
  const run = await conn.sendRequest("textDocument/codeLens", { textDocument: { uri: uri("hint.ev") } });
  ok(
    "a Run link at the start of a script",
    run.length === 1 && run[0].range.start.line === 0 && run[0].command.command === "evie.run" && run[0].command.arguments[1] === "hint.ev",
    run,
  );

  conn.sendNotification("textDocument/didOpen", { textDocument: { uri: uri("typo.ev"), languageId: "evie", version: 1, text: "value := 1\necho valeu\n" } });
  await waitFor("typo.ev's error", () => diagnostics.get(uri("typo.ev"))?.length > 0);
  const typo = diagnostics.get(uri("typo.ev"));
  const actions = await conn.sendRequest("textDocument/codeAction", {
    textDocument: { uri: uri("typo.ev") },
    range: typo[0].range,
    context: { diagnostics: typo },
  });
  ok("a quick fix for a misspelt name", actions?.[0]?.edit?.changes?.[uri("typo.ev")]?.[0]?.newText === "value", actions);

  await conn.sendRequest("shutdown");
  await Promise.resolve(conn.sendNotification("exit")).catch(() => {});
} catch (err) {
  failed++;
  console.log("FAIL", err.message);
} finally {
  conn.dispose();
  proc.kill();
}

// VS Code starts the server as a forked Node process and talks over its IPC
// channel rather than stdio; the server must work that way too
try {
  const { fork } = await import("node:child_process");
  const child = fork(server, ["--node-ipc"], { stdio: ["ignore", "ignore", "inherit", "ipc"] });
  const ipc = rpc.createMessageConnection(new rpc.IPCMessageReader(child), new rpc.IPCMessageWriter(child));
  let published = null;
  ipc.onNotification("textDocument/publishDiagnostics", (p) => (published = p));
  ipc.listen();
  await ipc.sendRequest("initialize", { processId: process.pid, rootUri: URI.file(root).toString(), capabilities: {} });
  ipc.sendNotification("initialized", {});
  ipc.sendNotification("textDocument/didOpen", {
    textDocument: { uri: uri("lib/b.ev"), languageId: "evie", version: 1, text: files["lib/b.ev"] },
  });
  const end = Date.now() + 10000;
  while (!published && Date.now() < end) await new Promise((r) => setTimeout(r, 50));
  ok("works over Node's IPC channel, as VS Code runs it", published !== null, published);
  ipc.dispose();
  child.kill();
} catch (err) {
  failed++;
  console.log("FAIL ipc", err.message);
}
process.exit(failed ? 1 : 0);
