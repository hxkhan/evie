# Evie for VS Code

Language support for [Evie](https://github.com/hxkhan/evie) (`.ev` files).

## Features

- **Errors as you type.** Every syntax error of a file, and the compile errors of every function, class and statement, checked across the files and packages a program imports. Nothing is run to find them.
- **Unused variables and imports are faded out.** A name that starts with `_` is unused on purpose and left alone.
- **Quick fixes** for misspelt names: `io.pritnln` offers `println`, from the names in scope or the package's members.
- **Go to definition** and **find all references**, across functions, closures, classes (inherited methods included) and files.
- **Go to Symbol in Workspace** (Ctrl+T) finds the functions, classes, methods and bindings of every file.
- **Import paths are links** to the file, or the folder's first file, they import.
- **Hover** shows how a name is declared: a function's parameters, a class's fields and parent, a binding's declaration. Standard library functions show what they do, and say when they **yield** (let other tasks run while they wait).
- **Completion** of the names in scope, keywords and builtin types; after `io.` the members of a package, after `this.` the fields and methods of the class.
- **Signature help** while a call is being typed, with the argument being written marked: functions, constructors, methods and the standard library.
- **Rename** a variable, parameter, function, class or import everywhere in the workspace. The renamed code is checked before the edit is made, so a rename that would collide with another name, or make code refer to something else, is refused with the reason.
- **Highlight occurrences** of the name under the cursor, its declaration marked as a write.
- **Folding** of every block, switch, and object or array literal that spans lines.
- **Outline** of a file's functions, classes and bindings, also while it has syntax errors.
- **Run** a program from the **▶ Run** link above `fn main` (or at the top of a script), the play button in the editor's title bar, or **Evie: Run** in the command palette. It saves your Evie files and runs `evie` in a terminal. A main file that is part of a project runs the whole folder (`evie .`). It needs `evie` on your PATH, or its location in the `evie.path` setting.
- Syntax highlighting, comment toggling, and auto-closing brackets, quotes and template backticks.

The analysis is Evie itself, compiled to WebAssembly and bundled with the extension, so nothing else needs to be installed except to run programs. Unsaved changes count: an edit to an imported file is checked before it is saved.

## How a file is checked

A file with `package main` is checked with the other `main` files of its folder, as a project, unless several of them declare `fn main`: then each is a program of its own, as in a folder of examples. A file of another package is checked with the rest of its folder when every file there declares the same package, and on its own otherwise. A file without a package header is a script. Imports are followed the way `evie` follows them.

## Developing

Needs Go and Node 20 or newer.

```bash
cd extensions/vscode
npm install
npm run build        # type check, bundle with esbuild, compile the analysis to WebAssembly
npm test             # drive the language server over the protocol on a sample workspace
npm run test:vscode  # run the extension inside a real VS Code (downloaded once into .vscode-test/)
npm run package      # build and write evie-<version>.vsix
```

Install the `.vsix` with **Extensions → … → Install from VSIX...**. To try the extension without packaging it, build it and run `code --extensionDevelopmentPath=.` from this folder.

The analysis lives in the repository's [`analysis`](../../analysis) package, which has its own Go tests; [`lsp/`](lsp) exposes it to the server in [`src/server.ts`](src/server.ts), and [`src/extension.ts`](src/extension.ts) starts the server.
