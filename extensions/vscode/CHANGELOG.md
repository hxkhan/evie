# Change Log

All notable changes to the "evie" extension will be documented in this file.

Check [Keep a Changelog](http://keepachangelog.com/) for recommendations on how to structure this file.

## [1.1.0]

- Added a language server: errors as you type, go to definition, find all references, hover, completion and an outline.
- The analysis is Evie's own compiler, bundled as WebAssembly; it checks a program and its imports without running anything, and sees unsaved edits.
- Hover on a standard library function shows what it does and whether it yields.
- Signature help, rename (checked before it is applied), highlighting of occurrences, and folding by block.
- Unused variables and imports are faded out, a misspelt name has a quick fix to the name the compiler suggests, Go to Symbol in Workspace finds declarations in every file, and import paths are links.
- A Run link above `fn main`, a play button and an **Evie: Run** command run the program with `evie` in a terminal; the `evie.path` setting says where evie is.
- Added highlighting for contextual `as` import aliases (`imports("./math" as mymath)`).
- Updated class syntax highlighting from `struct` to `class`, including class fields and `: Parent` inheritance scopes.
- Added highlighting for `const`, `super`, `await.timeout`, and built-in types (`buffer`, `array`, `object`, `task`).
- Removed obsolete keywords (`synced`, `unsynced`, `agnostic`, `pub`, `record`, `static`, `self`), non-existent operators (bitwise, `++`, `--`), single-quoted char literals, and unsupported numeric literal forms.
- Added highlighting for `=>` arrow functions, `...` variadic parameters, and `?` existence checks.
- Fixed template literal highlighting so escape sequences (`\n`, `\t`, `\r`, `\0`, `\\`, `\"`, `` \` ``, `\{`, `\}`) match before `{...}` interpolation and nested braces inside interpolations are handled cleanly.
- Updated `language-configuration.json` to auto-close and surround backtick template literals instead of single quotes.

## [1.0.0]

- Initial release
