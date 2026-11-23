// Builds the extension into out/: the client and the language server bundled
// with esbuild, Evie's analysis compiled to WebAssembly, and Go's loader for it.
//
//	node scripts/build.mjs
import { execFileSync } from "node:child_process";
import { copyFileSync, mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";

const here = dirname(fileURLToPath(import.meta.url));
const ext = resolve(here, "..");
const repo = resolve(ext, "../..");
const out = join(ext, "out");
mkdirSync(out, { recursive: true });

// type check first, since esbuild only strips types
execFileSync(process.execPath, [join(ext, "node_modules/typescript/bin/tsc"), "-p", ext], { stdio: "inherit" });

for (const name of ["extension", "server"]) {
  await build({
    entryPoints: [join(ext, "src", `${name}.ts`)],
    outfile: join(out, `${name}.js`),
    bundle: true,
    platform: "node",
    format: "cjs",
    target: "node20",
    external: ["vscode"],
    minify: true,
    logLevel: "warning",
  });
}

execFileSync("go", ["build", "-trimpath", "-ldflags=-s -w", "-o", join(out, "evie-lsp.wasm"), "./extensions/vscode/lsp"], {
  cwd: repo,
  env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
  stdio: "inherit",
});
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(join(goroot, "lib", "wasm", "wasm_exec.js"), join(out, "wasm_exec.js"));
console.log("built out/extension.js, out/server.js and out/evie-lsp.wasm");
