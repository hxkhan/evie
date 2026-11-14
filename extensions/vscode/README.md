# Evie for VS Code

Syntax highlighting and editor configuration for the [Evie](https://github.com/hxkhan/evie) programming language (`.ev` files).

## Features

- Syntax highlighting for Evie keywords, `class` definitions and inheritance, `fn` declarations, arrow functions, `go`/`await` concurrency (`await.all`, `await.any`, `await.race`, `await.timeout`), built-in types, and string template interpolation.
- Line (`//`) and block (`/* ... */`) comment toggling.
- Auto-closing and surrounding pairs for brackets, double quotes, and backtick template literals.

## Packaging

From this directory:

```bash
npx @vscode/vsce package
```

Then install the generated `.vsix` file via **Extensions → Install from VSIX...** in VS Code.
