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

// run runs code with all of its input given up front as stdin, or, with typed,
// the way the page runs it: each read the program asks for gets the next
// typed line, and the end of the input once there are none
function run(code, { stdin = "", typed } = {}) {
  return new Promise((resolve) => {
    let out = "";
    let reads = 0;
    const lines = [...(typed ?? [])];
    listener = (m) => {
      if (m.type === "out") out += m.text;
      if (m.type === "read") {
        reads++;
        if (lines.length > 0) globalThis.evieInput(lines.shift());
        else globalThis.evieEOF();
      }
      if (m.type === "done") resolve({ ...m, out, reads });
    };
    if (typed) globalThis.evieRun(code);
    else globalThis.evieRun(code, stdin);
  });
}

const cases = [
  { name: "expression", code: "1 + 2", check: (r) => r.result === "3" },
  { name: "program", code: 'package main imports("io")\nfn main() {\n  io.println("hi")\n  return 42\n}', check: (r) => r.out === "hi\n" && r.result === "42" },
  { name: "stdin", code: "x := io.readln()\nx.toUpper()", stdin: "evie\n", check: (r) => r.result === '"EVIE"' || r.result === "EVIE" },
  {
    name: "typed input",
    code: 'package main imports("io")\nfn main() {\n  name := io.prompt("name? ")\n  io.println(`hi {name}`)\n  io.println(io.readln())\n}',
    typed: ["Ada"],
    check: (r) => r.out === "name? hi Ada\nnil\n" && r.reads === 2,
  },
  { name: "error", code: "x := 1 +", check: (r) => typeof r.error === "string" && r.error.includes("error") },
  { name: "sleep", code: 'package main imports("time", "io")\nfn main() {\n  t := go time.sleep(20)\n  await t\n  io.println("woke")\n}', check: (r) => r.out === "woke\n" },
  { name: "fs", code: 'fs.readText("hello.txt").trim()', check: (r) => (r.result ?? "").includes("Hello from") },
];

// every example of the playground runs, with what a user would type, and says
// what it should
const typed = { temperatures: ["21C", "98.6F", ""] };
const expected = {
  hero: "Ada has 70 left",
  words: "times  ## 2",
  shapes: '["wheel", "washer"]',
  closures: "fib(70) = 190392490709135",
  errors: "total: 3.5",
  parallel: "caught: await.timeout",
  shared: "5 of 5 went through, leaving -50",
  temperatures: "98.6F is 37C",
  fib: "fib(30) = 832040",
};
const examples = JSON.parse(await readFile(join(here, "..", "src", "generated", "examples.json"), "utf8"));
for (const ex of examples) {
  cases.push({
    name: `example ${ex.id}`,
    code: ex.code,
    typed: typed[ex.id],
    check: (r) => !r.error && expected[ex.id] !== undefined && r.out.includes(expected[ex.id]),
  });
}

let failed = 0;
for (const c of cases) {
  const r = await run(c.code, c);
  const ok = c.check(r);
  if (!ok) failed++;
  console.log(`${ok ? "ok  " : "FAIL"} ${c.name}`, ok ? "" : JSON.stringify(r));
}

// the editor's help: errors as you type, completion and hover
const json = (s) => JSON.parse(s);
const errorsIn = (code) => json(globalThis.evieCheck(code)).filter((d) => d.severity === 1);
const help = [
  ["every example checks without errors", () => examples.every((ex) => errorsIn(ex.code).length === 0)],
  ["a snippet names io without importing it", () => errorsIn(["x := io.readln()", "io.println(x)"].join("\n")).length === 0],
  [
    "a misspelt member is an error with a fix",
    () => {
      const [d] = errorsIn(['package main imports("io")', "fn main() {", '    io.printn("hi")', "}"].join("\n"));
      return d?.range.start.line === 2 && d.data?.fixes[0]?.edit.newText === "print";
    },
  ],
  ["a syntax error is reported", () => errorsIn("x := 1 +").length === 1],
  ["an unused variable is a hint", () => json(globalThis.evieCheck(["fn f() {", "    unused := 1", "}"].join("\n"))).some((d) => d.severity === 4 && d.tags?.includes(1))],
  ["io. completes its members", () => json(globalThis.evieComplete(["x := 1", "io."].join("\n"), 1, 3)).some((c) => c.label === "println")],
  ["a name completes from what is in scope", () => json(globalThis.evieComplete(["total := 1", "to"].join("\n"), 1, 2)).some((c) => c.label === "total")],
  ["hover shows a stdlib function's docs", () => json(globalThis.evieHover('io.println("hi")', 0, 5))?.contents.includes("new line")],
];
for (const [name, test] of help) {
  const ok = test();
  if (!ok) failed++;
  console.log(`${ok ? "ok  " : "FAIL"} ${name}`);
}
process.exit(failed ? 1 : 0);
