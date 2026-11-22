import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

// every generated page is an entry: index.html, play/ and each guide page
function pages(dir: string, root = dir): Record<string, string> {
  const out: Record<string, string> = {};
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      if (["node_modules", "dist", "public", "src", "scripts", "wasm"].includes(name)) continue;
      Object.assign(out, pages(path, root));
    } else if (name === "index.html") {
      out[relative(root, dir).replaceAll("\\", "/") || "home"] = path;
    }
  }
  return out;
}

export default defineConfig({
  // a GitHub project page lives under /<repo>/; SITE_BASE says so at build time
  base: process.env.SITE_BASE ?? "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: { input: pages(fileURLToPath(new URL(".", import.meta.url))) },
  },
});
