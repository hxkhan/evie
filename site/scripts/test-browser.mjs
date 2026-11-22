// End-to-end check of the playground in headless Chrome: loads the page from
// a running preview server, waits for Evie to load, runs programs and reads
// the output. Needs Chrome and `npx vite preview --port 4173`.
//
//	node scripts/test-browser.mjs [chrome path]
import { spawn } from "node:child_process";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const chrome = process.argv[2] ?? "C:/Program Files/Google/Chrome/Application/chrome.exe";
const site = process.env.SITE_URL ?? "http://localhost:4173";
const port = 9333;
const profile = mkdtempSync(join(tmpdir(), "evie-chrome-"));
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, "about:blank"], { stdio: "ignore" });

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let target;
for (let i = 0; i < 50 && !target; i++) {
  await sleep(200);
  try {
    target = (await (await fetch(`http://127.0.0.1:${port}/json`)).json()).find((t) => t.type === "page");
  } catch {}
}
if (!target) throw new Error("Chrome did not start");

const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => ws.addEventListener("open", r, { once: true }));
let id = 0;
const pending = new Map();
const errors = [];
ws.addEventListener("message", (e) => {
  const msg = JSON.parse(e.data);
  if (msg.id && pending.has(msg.id)) {
    pending.get(msg.id)(msg);
    pending.delete(msg.id);
  }
  if (msg.method === "Runtime.exceptionThrown") errors.push(msg.params.exceptionDetails.text);
  if (msg.method === "Runtime.consoleAPICalled" && msg.params.type === "error") errors.push(msg.params.args.map((a) => a.value ?? a.description).join(" "));
});
const send = (method, params = {}) =>
  new Promise((resolve) => {
    const n = ++id;
    pending.set(n, resolve);
    ws.send(JSON.stringify({ id: n, method, params }));
  });
const evaluate = async (expression) => (await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true })).result?.result?.value;
const waitFor = async (expression, ms = 20000) => {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    if (await evaluate(expression)) return true;
    await sleep(100);
  }
  return false;
};

await send("Runtime.enable");
let failed = 0;
const check = (name, ok, detail = "") => {
  if (!ok) failed++;
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${ok ? "" : " " + detail}`);
};

async function runExample(hash, expectText, stdin) {
  await send("Page.navigate", { url: "about:blank" }); // a new hash alone would not reload the page
  await send("Page.navigate", { url: `${site}/play/${hash}` });
  const ready = await waitFor(`document.getElementById("status")?.textContent === "Ready"`);
  if (!ready) return { ready, text: await evaluate(`document.getElementById("status")?.textContent`) };
  if (stdin !== undefined) await evaluate(`document.getElementById("stdin").value = ${JSON.stringify(stdin)}`);
  await evaluate(`document.getElementById("run").click()`);
  await waitFor(`document.querySelector("#panel-output .meta") !== null`);
  const text = await evaluate(`document.getElementById("panel-output").innerText`);
  return { ready, text, ok: text.includes(expectText) };
}

// SHOTS=<dir> also saves screenshots of the playground after a run
async function shot(name) {
  if (!process.env.SHOTS) return;
  const { writeFileSync } = await import("node:fs");
  await send("Emulation.setDeviceMetricsOverride", { width: 1400, height: 860, deviceScaleFactor: 1, mobile: false });
  const res = await send("Page.captureScreenshot", { format: "png" });
  writeFileSync(join(process.env.SHOTS, `${name}.png`), Buffer.from(res.result.data, "base64"));
}

let r = await runExample("#example=hero", "Hello, Linus!");
check("loads Evie and runs the hero example", r.ready && r.ok, JSON.stringify(r));
await shot("play-run");
r = await runExample("#code=" + Buffer.from('package main imports("io")\nfn main() {\n    io.println("about to fail")\n    return lenght([1, 2])\n}\nfn length(a) => a.len()').toString("base64url"), "error");
await shot("play-error");

const code = (s) => "#code=" + Buffer.from(s).toString("base64url");
r = await runExample(code("name := io.readln()\n`Hi, {name}`"), "Hi, Grace", "Grace\n");
check("reads stdin and shows the result", r.ok, JSON.stringify(r));

r = await runExample(code("x := 1 +"), "error");
check("shows a syntax error", r.ok, JSON.stringify(r));

// a loop that never ends is stopped by terminating the worker, and Evie comes back
await send("Page.navigate", { url: "about:blank" });
await send("Page.navigate", { url: `${site}/play/${code("var i := 0\nwhile true { i += 1 }")}` });
await waitFor(`document.getElementById("status")?.textContent === "Ready"`);
await evaluate(`document.getElementById("run").click()`);
await sleep(500);
await evaluate(`document.getElementById("stop").click()`);
const back = await waitFor(`document.getElementById("status")?.textContent === "Ready"`);
check("stops a runaway loop and restarts", back);

for (const page of ["/", "/docs/", "/docs/classes-object-oriented-programming/"]) {
  await send("Page.navigate", { url: site + page });
  await sleep(800);
  const title = await evaluate("document.title");
  check(`renders ${page} (${title})`, !!title && !title.includes("404"));
}
check("no script errors", errors.length === 0, JSON.stringify(errors));

// phone-width screenshots, emulating a phone rather than a narrow window
if (process.env.SHOTS) {
  const { writeFileSync } = await import("node:fs");
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 1200, deviceScaleFactor: 1, mobile: true });
  for (const [name, page] of [["m-home", "/"], ["m-docs", "/docs/operators/"], ["m-play", "/play/#example=hero"]]) {
    await send("Page.navigate", { url: "about:blank" });
    await send("Page.navigate", { url: site + page });
    await sleep(1500);
    const overflow = await evaluate("document.documentElement.scrollWidth - innerWidth");
    check(`${page} fits a phone (overflow ${overflow}px)`, overflow <= 0);
    const res = await send("Page.captureScreenshot", { format: "png" });
    writeFileSync(join(process.env.SHOTS, `${name}.png`), Buffer.from(res.result.data, "base64"));
  }
}

ws.close();
proc.kill();
process.exit(failed ? 1 : 0);
