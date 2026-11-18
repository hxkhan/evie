# Change Log

All notable changes to the "evie" extension will be documented in this file.

Check [Keep a Changelog](http://keepachangelog.com/) for recommendations on how to structure this file.

## [Unreleased]

- Added highlighting for contextual `as` import aliases (`imports("./math" as mymath)`).
- Updated class syntax highlighting from `struct` to `class`, including class fields and `: Parent` inheritance scopes.
- Added highlighting for `const`, `super`, `await.timeout`, and built-in types (`buffer`, `array`, `object`, `task`).
- Removed obsolete keywords (`synced`, `unsynced`, `agnostic`, `pub`, `record`, `static`, `self`), non-existent operators (bitwise, `++`, `--`), single-quoted char literals, and unsupported numeric literal forms.
- Added highlighting for `=>` arrow functions, `...` variadic parameters, and `?` existence checks.
- Fixed template literal highlighting so escape sequences (`\n`, `\t`, `\r`, `\0`, `\\`, `\"`, `` \` ``, `\{`, `\}`) match before `{...}` interpolation and nested braces inside interpolations are handled cleanly.
- Updated `language-configuration.json` to auto-close and surround backtick template literals instead of single quotes.

## [1.0.0]

- Initial release
