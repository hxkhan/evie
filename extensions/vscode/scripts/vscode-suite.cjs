// The checks scripts/test-vscode.mjs runs inside VS Code, through VS Code's
// own commands, the way the editor asks the extension
const vscode = require("vscode");
const path = require("node:path");

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function until(what, check, ms = 30000) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    const value = await check();
    if (value) return value;
    await sleep(200);
  }
  throw new Error(`timed out waiting for ${what}`);
}

exports.run = async function run() {
  const root = vscode.workspace.workspaceFolders[0].uri.fsPath;
  const uri = (name) => vscode.Uri.file(path.join(root, name));
  const results = [];
  const ok = (name, cond, detail) => {
    results.push(`${cond ? "ok  " : "FAIL"} ${name}${cond ? "" : " " + JSON.stringify(detail)}`);
    if (!cond) results.failed = true;
  };

  // errors appear for a broken file once it is open
  await vscode.window.showTextDocument(await vscode.workspace.openTextDocument(uri("broken.ev")));
  const errors = await until("broken.ev's errors", () => {
    const d = vscode.languages.getDiagnostics(uri("broken.ev"));
    return d.length > 0 && d;
  });
  ok("errors appear in the editor", errors[0].message.includes("cannot find 'nope'"), errors.map((e) => e.message));

  const main = await vscode.workspace.openTextDocument(uri("main.ev"));
  await vscode.window.showTextDocument(main);
  await until("main.ev to be checked", async () => {
    const h = await vscode.commands.executeCommand("vscode.executeHoverProvider", main.uri, new vscode.Position(3, 9));
    return h && h.length > 0;
  });

  const defs = await vscode.commands.executeCommand("vscode.executeDefinitionProvider", main.uri, new vscode.Position(3, 23));
  const def = defs && defs[0];
  const target = def && (def.uri || def.targetUri);
  ok("go to definition opens the other file", target && target.fsPath.endsWith("util.ev"), defs);

  const hovers = await vscode.commands.executeCommand("vscode.executeHoverProvider", main.uri, new vscode.Position(3, 9));
  const text = hovers.map((h) => h.contents.map((c) => c.value ?? c).join("\n")).join("\n");
  ok("hover describes io.println", text.includes("Writes the values"), text);

  const folds = await vscode.commands.executeCommand("vscode.executeFoldingRangeProvider", main.uri);
  ok("folding by block", (folds ?? []).some((f) => f.start === 2 && f.end === 3), folds);

  const marks = await vscode.commands.executeCommand("vscode.executeDocumentHighlights", main.uri, new vscode.Position(3, 16));
  ok("highlight occurrences", (marks ?? []).length === 2, marks);

  const help = await vscode.commands.executeCommand("vscode.executeSignatureHelpProvider", main.uri, new vscode.Position(3, 26), "(");
  ok("signature help", help && help.signatures[0].label === "util.greet(n)", help);

  const renamed = await vscode.commands.executeCommand("vscode.executeDocumentRenameProvider", main.uri, new vscode.Position(3, 21), "welcome");
  const renamedFiles = renamed ? renamed.entries().map(([u]) => path.basename(u.fsPath)).sort().join(" ") : "";
  ok("rename across files", renamedFiles === "main.ev util.ev", renamedFiles);

  const symbols = await vscode.commands.executeCommand("vscode.executeWorkspaceSymbolProvider", "gree");
  ok("search the workspace's symbols", (symbols ?? []).some((s) => s.name === "greet" && s.location.uri.fsPath.endsWith("util.ev")), symbols);

  const links = await vscode.commands.executeCommand("vscode.executeLinkProvider", main.uri);
  ok("import paths are links", (links ?? []).some((l) => l.target && l.target.fsPath.endsWith("util.ev")), links);

  // a hint fades out an unused name
  await vscode.window.showTextDocument(await vscode.workspace.openTextDocument(uri("unused.ev")));
  const hints = await until("unused.ev's hint", () => {
    const d = vscode.languages.getDiagnostics(uri("unused.ev"));
    return d.length > 0 && d;
  });
  ok(
    "an unused name is faded out",
    hints[0].severity === vscode.DiagnosticSeverity.Hint && (hints[0].tags ?? []).includes(vscode.DiagnosticTag.Unnecessary),
    hints,
  );

  // a misspelt name has a quick fix
  const typo = await vscode.workspace.openTextDocument(uri("typo.ev"));
  await vscode.window.showTextDocument(typo);
  const typoErrors = await until("typo.ev's error", () => {
    const d = vscode.languages.getDiagnostics(typo.uri);
    return d.length > 0 && d;
  });
  const actions = await vscode.commands.executeCommand("vscode.executeCodeActionProvider", typo.uri, typoErrors[0].range);
  const fix = (actions ?? []).find((a) => a.title === "Change 'pritnln' to 'println'");
  ok("a quick fix for a misspelt name", fix && fix.edit, actions);
  if (fix && fix.edit) {
    await vscode.workspace.applyEdit(fix.edit);
    ok("the quick fix corrects it", typo.getText().includes("io.println(1)"), typo.getText());
  }

  // the Run link above fn main runs the program in a terminal
  await vscode.window.showTextDocument(main);
  const lenses = await vscode.commands.executeCommand("vscode.executeCodeLensProvider", main.uri);
  const runLens = (lenses ?? []).find((l) => l.command && l.command.command === "evie.run");
  ok("a Run link above fn main", runLens && runLens.range.start.line === 2 && runLens.command.arguments[1] === "main.ev", lenses);
  if (runLens) {
    const ended = new Promise((resolve) => {
      const sub = vscode.tasks.onDidEndTaskProcess((e) => {
        sub.dispose();
        resolve(e.exitCode);
      });
    });
    const execution = await vscode.commands.executeCommand(runLens.command.command, ...runLens.command.arguments);
    const code = await Promise.race([ended, sleep(30000).then(() => "timed out")]);
    ok("Run runs the program with evie", execution && code === 0, code);
  }

  const edit = new vscode.WorkspaceEdit();
  edit.insert(main.uri, new vscode.Position(4, 0), "    io.\n");
  await vscode.workspace.applyEdit(edit);
  const list = await vscode.commands.executeCommand("vscode.executeCompletionItemProvider", main.uri, new vscode.Position(4, 7), ".");
  const labels = list.items.map((i) => (typeof i.label === "string" ? i.label : i.label.label));
  ok("completion offers the members of io", labels.includes("println"), labels.slice(0, 20));

  console.log(results.join("\n"));
  if (results.failed) {
    throw new Error("some checks failed");
  }
};
