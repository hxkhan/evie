// The VS Code side of Evie's language support: it starts the language server
// (out/server.js) and connects it to the editor, and runs programs with evie.
import * as fs from "node:fs";
import * as path from "node:path";
import {
  commands,
  ExtensionContext,
  ProcessExecution,
  Task,
  TaskExecution,
  TaskPanelKind,
  TaskRevealKind,
  tasks,
  TaskScope,
  Uri,
  window,
  workspace,
} from "vscode";
import { LanguageClient, LanguageClientOptions, ServerOptions, TransportKind } from "vscode-languageclient/node";

let client: LanguageClient | undefined;

export async function activate(context: ExtensionContext) {
  const module = context.asAbsolutePath(path.join("out", "server.js"));
  const serverOptions: ServerOptions = {
    run: { module, transport: TransportKind.ipc },
    debug: { module, transport: TransportKind.ipc, options: { execArgv: ["--nolazy", "--inspect=6009"] } },
  };
  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "evie" }],
    synchronize: {
      // a change on disk can change what an open file imports
      fileEvents: workspace.createFileSystemWatcher("**/*.ev"),
    },
  };
  client = new LanguageClient("evie", "Evie", serverOptions, clientOptions);
  context.subscriptions.push(commands.registerCommand("evie.run", run));
  await client.start();
}

export function deactivate(): Thenable<void> | undefined {
  return client?.stop();
}

// run runs a program with evie in a terminal: arg (a file's name, or "." for
// a project) in the folder cwd, as the Run link above fn main gives them, or
// else the program the active file starts
async function run(cwd?: string, arg?: string): Promise<TaskExecution | undefined> {
  if (!cwd || !arg) {
    const doc = window.activeTextEditor?.document;
    if (!doc || doc.languageId !== "evie" || !client) {
      window.showInformationMessage("Open an Evie file to run it.");
      return;
    }
    const target: { cwd: string; arg: string } | null = await client.sendRequest("evie/runTarget", { uri: doc.uri.toString() });
    if (!target) {
      window.showInformationMessage("This file is not a program: a program is a script, or a main file that declares fn main.");
      return;
    }
    ({ cwd, arg } = target);
  }

  const evie = workspace.getConfiguration("evie").get<string>("path") || "evie";
  if (!findProgram(evie)) {
    const choice = await window.showErrorMessage(`Cannot find '${evie}' to run the program. Install Evie, or set evie.path.`, "Open Settings");
    if (choice) {
      commands.executeCommand("workbench.action.openSettings", "evie.path");
    }
    return;
  }

  // evie reads the files from disk
  for (const doc of workspace.textDocuments) {
    if (doc.languageId === "evie" && doc.isDirty) {
      await doc.save();
    }
  }

  const folder = workspace.getWorkspaceFolder(Uri.file(cwd));
  const task = new Task(
    { type: "evie", target: path.join(cwd, arg) },
    folder ?? TaskScope.Workspace,
    arg === "." ? `run ${path.basename(cwd)}` : `run ${arg}`,
    "evie",
    new ProcessExecution(evie, [arg], { cwd }),
  );
  task.presentationOptions = {
    reveal: TaskRevealKind.Always,
    panel: TaskPanelKind.Dedicated,
    clear: true,
    focus: true,
    showReuseMessage: false,
  };
  return tasks.executeTask(task);
}

// findProgram reports whether a program can be started: a path that exists, or
// a name found on PATH
function findProgram(program: string): boolean {
  const isFile = (p: string) => {
    try {
      return fs.statSync(p).isFile();
    } catch {
      return false;
    }
  };
  if (program.includes("/") || program.includes("\\")) {
    return isFile(program);
  }
  const exts = process.platform === "win32" ? (process.env.PATHEXT ?? ".EXE;.CMD;.BAT").split(";").concat("") : [""];
  for (const dir of (process.env.PATH ?? "").split(path.delimiter)) {
    for (const ext of exts) {
      if (dir && isFile(path.join(dir, program + ext))) {
        return true;
      }
    }
  }
  return false;
}
