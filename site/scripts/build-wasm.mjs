// Builds the playground's interpreter: Evie compiled to WebAssembly, plus the
// wasm_exec.js glue from the Go toolchain that loads it.
//
//	node scripts/build-wasm.mjs
import { execFileSync } from "node:child_process";
import { copyFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const site = fileURLToPath(new URL("..", import.meta.url));
const repo = join(site, "..");
const out = join(site, "public");
mkdirSync(out, { recursive: true });

const env = { ...process.env, GOOS: "js", GOARCH: "wasm" };
execFileSync("go", ["build", "-trimpath", "-ldflags=-s -w", "-o", join(out, "evie.wasm"), "./site/wasm"], {
  cwd: repo,
  env,
  stdio: "inherit",
});

const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(join(goroot, "lib", "wasm", "wasm_exec.js"), join(out, "wasm_exec.js"));
console.log("built public/evie.wasm and public/wasm_exec.js");
