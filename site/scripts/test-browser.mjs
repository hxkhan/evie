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

// press sends a key to whatever has the focus, the way a keyboard does
const press = (key, code, keyCode, modifiers = 0) =>
  send("Input.dispatchKeyEvent", { type: "keyDown", key, code, windowsVirtualKeyCode: keyCode, modifiers }).then(() =>
    send("Input.dispatchKeyEvent", { type: "keyUp", key, code, windowsVirtualKeyCode: keyCode, modifiers }),
  );

// runExample runs the playground's code at hash, typing each of typed when the
// program waits for a line: a string is typed and entered, null is Ctrl+D
async function runExample(hash, expectText, typed = []) {
  await send("Page.navigate", { url: "about:blank" }); // a new hash alone would not reload the page
  await send("Page.navigate", { url: `${site}/play/${hash}` });
  const ready = await waitFor(`document.getElementById("status")?.textContent === "Ready"`);
  if (!ready) return { ready, text: await evaluate(`document.getElementById("status")?.textContent`) };
  await evaluate(`document.getElementById("run").click()`);
  for (const line of typed) {
    // the field for the line appears in the output and takes the focus
    const asked = await waitFor(`document.activeElement?.classList.contains("term-input")`);
    if (!asked) return { ready, asked, text: await evaluate(`document.getElementById("panel-output").innerText`) };
    if (line === null) {
      await press("d", "KeyD", 68, 2);
    } else {
      await send("Input.insertText", { text: line });
      await press("Enter", "Enter", 13);
    }
  }
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

let r = await runExample("#example=hero", "Ada has 70 left");
check("loads Evie and runs the hero example", r.ready && r.ok, JSON.stringify(r));

// the example picker opens with the keyboard, moves, and loads what is chosen
await evaluate(`document.querySelector(".picker__button").focus()`);
await press("ArrowDown", "ArrowDown", 40);
const opened = await evaluate(`!document.getElementById("example-menu").hidden && document.activeElement?.id === "example-menu"`);
await press("End", "End", 35);
await press("Enter", "Enter", 13);
const picked = await evaluate(
  `[document.getElementById("example-menu").hidden, document.getElementById("example-current").textContent, location.hash, document.querySelector(".cm-content").innerText.includes("fn fib(n)")]`,
);
check("the example picker works from the keyboard", opened && JSON.stringify(picked) === JSON.stringify([true, "Fibonacci", "#example=fib", true]), JSON.stringify({ opened, picked }));
await evaluate(`document.querySelector(".picker__button").click()`);
await shot("play-picker");
await evaluate(`document.querySelector(".picker__button").click()`);
await shot("play-run");
r = await runExample("#example=temperatures", "> 98.6F\n98.6F is 37C\n> \nBye!", ["98.6F", ""]);
check("types the lines a program reads into the output, as in a terminal", r.ready && r.ok, JSON.stringify(r));
await shot("play-input");
r = await runExample("#code=" + Buffer.from('package main imports("io")\nfn main() {\n    io.println("about to fail")\n    return lenght([1, 2])\n}\nfn length(a) => a.len()').toString("base64url"), "error");
await shot("play-error");

const code = (s) => "#code=" + Buffer.from(s).toString("base64url");
r = await runExample(code("name := io.readln()\nrest := io.readln()\n`Hi, {name} {rest}`"), "Hi, Grace nil", ["Grace", null]);
check("Ctrl+D ends the input", r.ok, JSON.stringify(r));

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

// on a Mac the Run shortcut reads ⌘ ↵, elsewhere Ctrl ↵
const runKey = `document.querySelector("#run kbd").textContent`;
const elsewhere = await evaluate(runKey);
await send("Emulation.setUserAgentOverride", {
  userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
  platform: "MacIntel",
  userAgentMetadata: { platform: "macOS", platformVersion: "14.0.0", architecture: "arm", model: "", mobile: false },
});
await send("Page.navigate", { url: `${site}/play/` });
await waitFor(`document.getElementById("status")?.textContent === "Ready"`);
const onMac = await evaluate(runKey);
await send("Emulation.setUserAgentOverride", { userAgent: "" });
check("the Run shortcut says ⌘ on a Mac and Ctrl elsewhere", elsewhere === "Ctrl ↵" && onMac === "⌘ ↵", JSON.stringify({ elsewhere, onMac }));

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
