// Runs the extension inside a real VS Code, which it downloads once into
// .vscode-test/, on a sample workspace, and checks what the editor shows:
// errors, hints and quick fixes, and what each request of the language server
// gives.
//
//	npm run build && node scripts/test-vscode.mjs
import { mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { runTests } from "@vscode/test-electron";

const here = dirname(fileURLToPath(import.meta.url));
const ext = resolve(here, "..");

const workspace = mkdtempSync(join(tmpdir(), "evie-vscode-"));
const files = {
  "main.ev": 'package main imports("io", "./util")\n\nfn main() {\n    io.println(util.greet(1))\n}\n',
  "util.ev": 'package util\nfn greet(n) => "hi " + string(n)\n',
  "broken.ev": "package main\nfn main() {\n    return nope\n}\n",
  "typo.ev": 'package main imports("io")\nfn main() { io.pritnln(1) }\n',
  "unused.ev": "x := 1\n",
};
for (const [name, text] of Object.entries(files)) {
  mkdirSync(dirname(join(workspace, name)), { recursive: true });
  writeFileSync(join(workspace, name), text);
}

// the Run button runs evie, built from this repository for the test
const evie = join(mkdtempSync(join(tmpdir(), "evie-bin-")), process.platform === "win32" ? "evie.exe" : "evie");
const built = spawnSync("go", ["build", "-o", evie, "./evie"], { cwd: resolve(ext, "..", ".."), stdio: "inherit" });
if (built.status !== 0) {
  console.error("building evie failed");
  process.exit(1);
}
mkdirSync(join(workspace, ".vscode"));
writeFileSync(join(workspace, ".vscode", "settings.json"), JSON.stringify({ "evie.path": evie }));

// started from inside VS Code, as from its terminal, the variable that makes
// Electron run as plain Node is set; the VS Code under test must not inherit it
delete process.env.ELECTRON_RUN_AS_NODE;

try {
  await runTests({
    version: "stable",
    extensionDevelopmentPath: ext,
    extensionTestsPath: join(here, "vscode-suite.cjs"),
    launchArgs: [workspace, "--disable-extensions", "--skip-welcome", "--skip-release-notes"],
  });
} catch (err) {
  console.error("the VS Code test failed:", err);
  process.exit(1);
}
