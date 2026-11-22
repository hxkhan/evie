// Runs the playground's evie.wasm under Node, the way the browser worker does,
// and checks a few programs. Build the wasm first: npm run wasm
//
//	node scripts/test-wasm.mjs
import { readFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
await import(pathToFileURL(join(goroot, "lib", "wasm", "wasm_exec.js")).href);

let listener = null;
globalThis.postMessage = (msg) => listener?.(msg);

const go = new globalThis.Go();
const bytes = await readFile(join(here, "..", "public", "evie.wasm"));
const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
const ready = new Promise((resolve) => (listener = (m) => m.type === "ready" && resolve(m)));
go.run(instance);
await ready;

function run(code, stdin = "") {
  return new Promise((resolve) => {
    let out = "";
    listener = (m) => {
      if (m.type === "out") out += m.text;
      if (m.type === "done") resolve({ ...m, out });
    };
    globalThis.evieRun(code, stdin);
  });
}

const cases = [
  { name: "expression", code: "1 + 2", check: (r) => r.result === "3" },
  { name: "program", code: 'package main imports("io")\nfn main() {\n  io.println("hi")\n  return 42\n}', check: (r) => r.out === "hi\n" && r.result === "42" },
  { name: "stdin", code: "x := io.readln()\nx.toUpper()", stdin: "evie\n", check: (r) => r.result === '"EVIE"' || r.result === "EVIE" },
  { name: "error", code: "x := 1 +", check: (r) => typeof r.error === "string" && r.error.includes("error") },
  { name: "sleep", code: 'package main imports("time", "io")\nfn main() {\n  t := go time.sleep(20)\n  await t\n  io.println("woke")\n}', check: (r) => r.out === "woke\n" },
  { name: "fs", code: 'fs.readText("hello.txt").trim()', check: (r) => (r.result ?? "").includes("Hello from") },
];

let failed = 0;
for (const c of cases) {
  const r = await run(c.code, c.stdin);
  const ok = c.check(r);
  if (!ok) failed++;
  console.log(`${ok ? "ok  " : "FAIL"} ${c.name}`, ok ? "" : JSON.stringify(r));
}
process.exit(failed ? 1 : 0);
