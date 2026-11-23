// Evie's language server. The Language Server Protocol is handled here, with
// vscode-languageserver; the analysis itself is Evie's Go code compiled to
// WebAssembly (evie-lsp.wasm), which reads files through evieHost so that open
// documents are seen as they are being edited.
import * as fs from "node:fs";
import * as path from "node:path";
import {
  CodeAction,
  CodeActionKind,
  CompletionItem,
  CompletionItemKind,
  Connection,
  createConnection,
  Diagnostic,
  DocumentLink,
  DocumentSymbol,
  Hover,
  InitializeResult,
  Location,
  MarkupKind,
  LSPErrorCodes,
  ProposedFeatures,
  ResponseError,
  TextDocuments,
  SymbolInformation,
  TextDocumentSyncKind,
} from "vscode-languageserver/node";
import { TextDocument } from "vscode-languageserver-textdocument";
import { URI } from "vscode-uri";

// the functions the WebAssembly module sets on globalThis.evieAnalysis; every
// result is JSON
interface Analysis {
  update(root: string, file: string): string;
  symbols(root: string, file: string): string;
  definition(root: string, file: string, line: number, character: number): string;
  references(root: string, file: string, line: number, character: number, includeDeclaration: boolean): string;
  hover(root: string, file: string, line: number, character: number): string;
  complete(root: string, file: string, line: number, character: number, text: string): string;
  folding(root: string, file: string): string;
  highlights(root: string, file: string, line: number, character: number): string;
  signatureHelp(root: string, file: string, line: number, character: number, text: string): string;
  prepareRename(root: string, file: string, line: number, character: number): string;
  rename(root: string, file: string, line: number, character: number, newName: string): string;
  workspaceSymbols(root: string, query: string): string;
  links(root: string, file: string): string;
  runnable(root: string, file: string): string;
  forget(root: string): void;
}

interface WireLocation {
  path: string;
  range: Diagnostic["range"];
}

interface WireDiagnostic extends Omit<Diagnostic, "relatedInformation"> {
  relatedInformation?: { location: WireLocation; message: string }[];
}

const connection: Connection = createConnection(ProposedFeatures.all);
const documents = new TextDocuments(TextDocument);

// the workspace folders, as file system paths; a file outside all of them is
// analyzed with its own folder as the root
let roots: string[] = [];

// a path as a key: Windows paths compare without regard to case
function key(p: string): string {
  const n = path.normalize(p);
  return process.platform === "win32" ? n.toLowerCase() : n;
}

function fsPathOf(uri: string): string {
  return URI.parse(uri).fsPath;
}

// rootOf is the workspace root a file belongs to and its slash-separated path in it
function rootOf(fsPath: string): { root: string; file: string } {
  let best = "";
  for (const r of roots) {
    const rel = path.relative(r, fsPath);
    if (!rel.startsWith("..") && !path.isAbsolute(rel) && r.length > best.length) {
      best = r;
    }
  }
  const root = best || path.dirname(fsPath);
  return { root, file: path.relative(root, fsPath).split(path.sep).join("/") };
}

// the open documents by their file system path
const open = new Map<string, TextDocument>();

// uriOf is how a file of a root is named to VS Code: an open document by its
// own URI, any other file the way VS Code writes file URIs
function uriOf(root: string, file: string): string {
  const full = path.join(root, ...file.split("/"));
  return open.get(key(full))?.uri ?? URI.file(full).toString();
}

function toLocation(root: string, loc: WireLocation): Location {
  return { uri: uriOf(root, loc.path), range: loc.range };
}

// ---------------------------------------------------------------- the host the analysis reads through

(globalThis as any).evieHost = {
  readFile(root: string, file: string): string | null {
    const full = path.join(root, ...file.split("/"));
    const doc = open.get(key(full));
    if (doc) {
      return doc.getText();
    }
    try {
      return fs.readFileSync(full, "utf8");
    } catch {
      return null;
    }
  },
  readDir(root: string, dir: string): { name: string; dir: boolean }[] | null {
    try {
      return fs
        .readdirSync(path.join(root, ...dir.split("/")), { withFileTypes: true })
        .map((e) => ({ name: e.name, dir: e.isDirectory() }));
    } catch {
      return null;
    }
  },
  stat(root: string, file: string): { dir: boolean } | null {
    const full = path.join(root, ...file.split("/"));
    if (open.has(key(full))) {
      return { dir: false };
    }
    try {
      return { dir: fs.statSync(full).isDirectory() };
    } catch {
      return null;
    }
  },
  ready() {
    resolveReady();
  },
};

let resolveReady: () => void;
const ready = new Promise<void>((resolve) => (resolveReady = resolve));

// a stand-in for the file system wasm_exec.js expects: the analysis touches no
// files through it, and anything Go writes goes to stderr, never to stdout,
// which may carry the protocol
function goFS() {
  const enosys = () => Object.assign(new Error("not implemented"), { code: "ENOSYS" });
  return {
    constants: { O_WRONLY: -1, O_RDWR: -1, O_CREAT: -1, O_TRUNC: -1, O_APPEND: -1, O_EXCL: -1, O_DIRECTORY: -1 },
    writeSync(_fd: number, buf: Uint8Array) {
      process.stderr.write(buf);
      return buf.length;
    },
    write(fd: number, buf: Uint8Array, offset: number, length: number, position: number | null, callback: Function) {
      if (offset !== 0 || length !== buf.length || position !== null) {
        callback(enosys());
        return;
      }
      callback(null, this.writeSync(fd, buf));
    },
    ...Object.fromEntries(
      ["chmod", "chown", "close", "fchmod", "fchown", "fstat", "fsync", "ftruncate", "lchown", "link", "lstat", "mkdir", "open", "read", "readdir", "readlink", "rename", "rmdir", "stat", "symlink", "truncate", "unlink", "utimes"].map(
        (name) => [name, (...args: any[]) => args[args.length - 1](enosys())],
      ),
    ),
  };
}

async function loadAnalysis(): Promise<Analysis> {
  (globalThis as any).fs = goFS();
  require(path.join(__dirname, "wasm_exec.js"));
  const go = new (globalThis as any).Go();
  const wasm = fs.readFileSync(path.join(__dirname, "evie-lsp.wasm"));
  const { instance } = await (globalThis as any).WebAssembly.instantiate(wasm, go.importObject);
  go.run(instance).then(() => connection.console.error("Evie's analysis stopped"));
  await ready;
  return (globalThis as any).evieAnalysis as Analysis;
}

const analysis = loadAnalysis();

// ---------------------------------------------------------------- diagnostics

// the files each analysis published diagnostics for, so it can clear them
const published = new Map<string, Set<string>>();

// the version of each document last checked, so one is not checked twice
const checked = new Map<string, number>();

async function check(doc: TextDocument) {
  checked.set(doc.uri, doc.version);
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(doc.uri));
  let result: Record<string, WireDiagnostic[]>;
  try {
    result = JSON.parse(api.update(root, file));
  } catch (err) {
    connection.console.error(`analyzing ${file} failed: ${err}`);
    return;
  }
  const entry = key(path.join(root, file));
  const now = new Set<string>();
  for (const [f, diags] of Object.entries(result)) {
    const uri = uriOf(root, f);
    now.add(uri);
    connection.sendDiagnostics({
      uri,
      diagnostics: diags.map((d) => ({
        ...d,
        relatedInformation: d.relatedInformation?.map((r) => ({ location: toLocation(root, r.location), message: r.message })),
      })),
    });
  }
  for (const uri of published.get(entry) ?? []) {
    if (!now.has(uri)) {
      connection.sendDiagnostics({ uri, diagnostics: [] });
    }
  }
  published.set(entry, now);
}

// changes are checked once typing pauses
const pending = new Map<string, NodeJS.Timeout>();
function checkSoon(doc: TextDocument, delay = 200) {
  clearTimeout(pending.get(doc.uri));
  pending.set(
    doc.uri,
    setTimeout(() => {
      pending.delete(doc.uri);
      const current = documents.get(doc.uri);
      if (current) {
        void check(current);
      }
    }, delay),
  );
}

documents.onDidOpen((e) => {
  open.set(key(fsPathOf(e.document.uri)), e.document);
  void check(e.document);
});
documents.onDidChangeContent((e) => {
  open.set(key(fsPathOf(e.document.uri)), e.document);
  // opening a document reports it as changed too, right after it was checked
  if (checked.get(e.document.uri) !== e.document.version) {
    checkSoon(e.document);
  }
});
documents.onDidSave((e) => void check(e.document));
documents.onDidClose((e) => {
  open.delete(key(fsPathOf(e.document.uri)));
  checked.delete(e.document.uri);
  clearTimeout(pending.get(e.document.uri));
});

// a file that changed on disk can change what every open file imports
connection.onDidChangeWatchedFiles(() => {
  for (const doc of documents.all()) {
    checkSoon(doc, 50);
  }
});

// ---------------------------------------------------------------- requests

connection.onInitialize((params): InitializeResult => {
  roots = (params.workspaceFolders ?? []).map((f) => fsPathOf(f.uri));
  if (roots.length === 0 && params.rootUri) {
    roots = [fsPathOf(params.rootUri)];
  }
  return {
    capabilities: {
      textDocumentSync: TextDocumentSyncKind.Incremental,
      definitionProvider: true,
      referencesProvider: true,
      hoverProvider: true,
      documentSymbolProvider: true,
      completionProvider: { triggerCharacters: ["."] },
      signatureHelpProvider: { triggerCharacters: ["(", ","], retriggerCharacters: [","] },
      foldingRangeProvider: true,
      documentHighlightProvider: true,
      renameProvider: { prepareProvider: true },
      workspaceSymbolProvider: true,
      documentLinkProvider: { resolveProvider: false },
      codeActionProvider: { codeActionKinds: [CodeActionKind.QuickFix] },
      codeLensProvider: { resolveProvider: false },
    },
    serverInfo: { name: "evie" },
  };
});

connection.onDefinition(async (params) => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  const loc: WireLocation | null = JSON.parse(api.definition(root, file, params.position.line, params.position.character));
  return loc ? toLocation(root, loc) : null;
});

connection.onReferences(async (params) => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  const locs: WireLocation[] | null = JSON.parse(
    api.references(root, file, params.position.line, params.position.character, params.context.includeDeclaration),
  );
  return (locs ?? []).map((l) => toLocation(root, l));
});

connection.onHover(async (params): Promise<Hover | null> => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  const h: { contents: string; range: Hover["range"] } | null = JSON.parse(
    api.hover(root, file, params.position.line, params.position.character),
  );
  return h ? { contents: { kind: MarkupKind.Markdown, value: h.contents }, range: h.range } : null;
});

connection.onDocumentSymbol(async (params): Promise<DocumentSymbol[]> => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  return JSON.parse(api.symbols(root, file)) ?? [];
});

connection.onCompletion(async (params): Promise<CompletionItem[]> => {
  const api = await analysis;
  const doc = documents.get(params.textDocument.uri);
  if (!doc) {
    return [];
  }
  const { root, file } = rootOf(fsPathOf(doc.uri));
  const items: { label: string; kind: CompletionItemKind; detail?: string; documentation?: string }[] =
    JSON.parse(api.complete(root, file, params.position.line, params.position.character, doc.getText())) ?? [];
  return items.map((c) => ({
    ...c,
    documentation: c.documentation ? { kind: MarkupKind.Markdown, value: c.documentation } : undefined,
  }));
});

connection.onSignatureHelp(async (params) => {
  const api = await analysis;
  const doc = documents.get(params.textDocument.uri);
  if (!doc) {
    return null;
  }
  const { root, file } = rootOf(fsPathOf(doc.uri));
  const help = JSON.parse(api.signatureHelp(root, file, params.position.line, params.position.character, doc.getText()));
  if (!help) {
    return null;
  }
  for (const s of help.signatures) {
    s.documentation = s.documentation ? { kind: MarkupKind.Markdown, value: s.documentation } : undefined;
  }
  return help;
});

connection.onFoldingRanges(async (params) => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  return JSON.parse(api.folding(root, file)) ?? [];
});

connection.onDocumentHighlight(async (params) => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  return JSON.parse(api.highlights(root, file, params.position.line, params.position.character)) ?? [];
});

connection.onPrepareRename(async (params) => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  const r = JSON.parse(api.prepareRename(root, file, params.position.line, params.position.character));
  if (r.error) {
    throw new ResponseError(LSPErrorCodes.RequestFailed, r.error);
  }
  return r;
});

connection.onRenameRequest(async (params) => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  const r = JSON.parse(api.rename(root, file, params.position.line, params.position.character, params.newName));
  if (r.error) {
    throw new ResponseError(LSPErrorCodes.RequestFailed, r.error);
  }
  const changes: Record<string, { range: Diagnostic["range"]; newText: string }[]> = {};
  for (const [f, edits] of Object.entries(r.changes as Record<string, { range: Diagnostic["range"]; newText: string }[]>)) {
    changes[uriOf(root, f)] = edits;
  }
  return { changes };
});

connection.onWorkspaceSymbol(async (params): Promise<SymbolInformation[]> => {
  const api = await analysis;
  const found: SymbolInformation[] = [];
  for (const root of roots) {
    const symbols: (Omit<SymbolInformation, "location"> & { location: WireLocation })[] =
      JSON.parse(api.workspaceSymbols(root, params.query)) ?? [];
    for (const s of symbols) {
      found.push({ ...s, location: toLocation(root, s.location) });
    }
  }
  return found;
});

connection.onDocumentLinks(async (params): Promise<DocumentLink[]> => {
  const api = await analysis;
  const { root, file } = rootOf(fsPathOf(params.textDocument.uri));
  const links: { range: DocumentLink["range"]; target: string; tooltip?: string }[] = JSON.parse(api.links(root, file)) ?? [];
  return links.map((l) => ({ ...l, target: uriOf(root, l.target) }));
});

// the quick fixes the analysis attached to the diagnostics in range, such as
// a name the compiler suggests for one it cannot find
connection.onCodeAction((params): CodeAction[] => {
  const actions: CodeAction[] = [];
  for (const d of params.context.diagnostics) {
    const fixes: { title: string; edit: { range: Diagnostic["range"]; newText: string } }[] = (d.data as any)?.fixes ?? [];
    for (const f of fixes) {
      actions.push({
        title: f.title,
        kind: CodeActionKind.QuickFix,
        diagnostics: [d],
        isPreferred: true,
        edit: { changes: { [params.textDocument.uri]: [f.edit] } },
      });
    }
  }
  return actions;
});

// ---------------------------------------------------------------- running

// RunTarget is how to run the program a file starts: evie is given arg in the
// folder cwd, the file's name or, for a project, "."
interface RunTarget {
  cwd: string;
  arg: string;
  range: Diagnostic["range"];
}

async function runTarget(uri: string): Promise<RunTarget | null> {
  const api = await analysis;
  const full = fsPathOf(uri);
  const { root, file } = rootOf(full);
  const r: { range: Diagnostic["range"]; target: string; project: boolean } | null = JSON.parse(api.runnable(root, file));
  if (!r) {
    return null;
  }
  return { cwd: path.dirname(full), arg: r.project ? "." : path.basename(full), range: r.range };
}

// a Run link above fn main, or at the start of a script; the extension runs it
connection.onCodeLens(async (params) => {
  const t = await runTarget(params.textDocument.uri);
  if (!t) {
    return [];
  }
  const title = t.arg === "." ? "▶ Run project" : "▶ Run";
  return [{ range: t.range, command: { title, command: "evie.run", arguments: [t.cwd, t.arg] } }];
});

// evie/runTarget: what the Run command runs for a document, or null
connection.onRequest("evie/runTarget", (params: { uri: string }) => runTarget(params.uri));

connection.onShutdown(async () => {
  const api = await analysis;
  for (const r of roots) {
    api.forget(r);
  }
});

documents.listen(connection);
connection.listen();
