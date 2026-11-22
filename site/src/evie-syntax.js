// Evie's syntax for highlighting, shared by the docs (highlighted at build
// time) and the playground's editor, so code looks the same in both. It has
// no dependencies, so the build scripts can import it under Node.

export const keywords = new Set([
  "package", "imports", "as",
  "var", "const", "fn", "class", "super", "this",
  "echo", "return",
  "if", "else", "switch", "case", "default",
  "is", "await", "go", "catch", "throw",
  "while", "break", "continue", "for", "in",
]);

export const atoms = new Set(["nil", "true", "false"]);

// the prelude's types and the standard packages
export const builtins = new Set([
  "string", "number", "bool", "array", "object", "buffer", "task", "error",
  "io", "fs", "math", "json", "time",
]);

// Go, for the host code in the guide: its own words, and backticks are raw
// strings rather than templates
const go = {
  keywords: new Set([
    "break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for", "func",
    "go", "goto", "if", "import", "interface", "map", "package", "range", "return", "select", "struct",
    "switch", "type", "var",
  ]),
  atoms: new Set(["nil", "true", "false", "iota"]),
  builtins: new Set([
    "append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min", "new",
    "panic", "print", "println", "real", "recover",
    "any", "bool", "byte", "comparable", "complex64", "complex128", "error", "float32", "float64", "int", "int8",
    "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
  ]),
};
const evie = { keywords, atoms, builtins };

/**
 * Tokenizes source into [text, type] pairs; type is "" for plain text.
 * lang is "evie" (the default) or "go".
 * Types: keyword, atom, builtin, number, string, comment, function, type,
 * operator, punctuation, property.
 */
export function tokenize(src, lang = "evie") {
  const out = [];
  const state = startState(lang);
  const stream = new LineStream(src);
  while (!stream.eof()) {
    const start = stream.pos;
    const type = token(stream, state) || "";
    if (stream.pos === start) stream.pos++; // never stall
    out.push([src.slice(start, stream.pos), type]);
  }
  return out;
}

export function startState(lang = "evie") {
  return { inComment: false, inTemplate: 0, inRaw: false, braces: [], prev: "", go: lang === "go" };
}

/**
 * Reads one token from a stream that has CodeMirror's StringStream shape
 * (pos, string, match, peek, next, eol) and returns its type.
 */
export function token(stream, state) {
  if (state.inComment) {
    const end = stream.string.indexOf("*/", stream.pos);
    if (end < 0) {
      stream.pos = stream.string.length;
    } else {
      stream.pos = end + 2;
      state.inComment = false;
    }
    return "comment";
  }
  if (state.inRaw) return rawText(stream, state);
  if (state.inTemplate > 0 && state.braces[state.braces.length - 1] === "`") {
    return templateText(stream, state);
  }

  if (stream.match(/^\s+/)) return null;
  if (stream.match("//")) {
    while (!stream.eol()) stream.next();
    return "comment";
  }
  if (stream.match("/*")) {
    state.inComment = true;
    return token(stream, state) && "comment";
  }

  const ch = stream.peek();
  if (ch === '"') {
    stream.next();
    let escaped = false;
    while (!stream.eol()) {
      const c = stream.next();
      if (escaped) escaped = false;
      else if (c === "\\") escaped = true;
      else if (c === '"') break;
    }
    state.prev = "string";
    return "string";
  }
  if (ch === "`" && state.go) {
    stream.next();
    state.inRaw = true;
    return rawText(stream, state);
  }
  if (ch === "`") {
    stream.next();
    state.inTemplate++;
    state.braces.push("`");
    return "string";
  }
  if (ch === "{" || ch === "(" || ch === "[") {
    stream.next();
    state.braces.push(ch);
    state.prev = ch;
    return "punctuation";
  }
  if (ch === "}" || ch === ")" || ch === "]") {
    stream.next();
    state.braces.pop();
    state.prev = ch;
    // the end of a template's interpolation resumes the template text
    return state.inTemplate > 0 && ch === "}" && state.braces[state.braces.length - 1] === "`" ? "string" : "punctuation";
  }
  if (stream.match(/^\d+(\.\d+)?/)) {
    state.prev = "number";
    return "number";
  }
  const word = stream.match(/^[A-Za-z_][A-Za-z0-9_]*/);
  if (word) {
    const w = word[0];
    const words = state.go ? go : evie;
    const afterDot = state.prev === ".";
    const declaresClass = state.prev === (state.go ? "type" : "class");
    const declares = state.prev === (state.go ? "func" : "fn") || declaresClass;
    state.prev = w;
    if (afterDot) return /^\s*\(/.test(stream.string.slice(stream.pos)) ? "function" : "property";
    if (words.keywords.has(w)) return "keyword";
    if (words.atoms.has(w)) return "atom";
    if (declares) return declaresClass ? "type" : "function";
    if (words.builtins.has(w)) return "builtin";
    // a field name in a Go composite literal: Options{Packages: ...}
    if (state.go && /^\s*:(?!=)/.test(stream.string.slice(stream.pos))) return "property";
    if (/^\s*\(/.test(stream.string.slice(stream.pos))) return /^[A-Z]/.test(w) ? "type" : "function";
    if (/^[A-Z]/.test(w)) return "type";
    return null;
  }
  if (stream.match(/^(:=|=>|==|!=|<=|>=|&&|\|\||\+=|-=|\*=|\/=|%=|\.\.\.|[-+*/%<>=!?:])/)) {
    state.prev = "op";
    return "operator";
  }
  const c = stream.next();
  state.prev = c;
  return c === "." || c === "," || c === ";" ? "punctuation" : null;
}

// rawText reads a Go raw string up to and including its closing backtick,
// which can be lines away
function rawText(stream, state) {
  while (!stream.eol()) {
    if (stream.next() === "`") {
      state.inRaw = false;
      break;
    }
  }
  return "string";
}

// templateText reads template text up to an interpolation or the closing
// backtick; an opening brace or the backtick is a token of its own
function templateText(stream, state) {
  const start = stream.pos;
  if (stream.peek() === "{") {
    stream.next();
    state.braces.push("{");
    return "string";
  }
  while (!stream.eol()) {
    const c = stream.peek();
    if (c === "\\") {
      stream.next();
      if (!stream.eol()) stream.next();
      continue;
    }
    if (c === "`") {
      if (stream.pos === start) {
        stream.next();
        state.inTemplate--;
        state.braces.pop();
      }
      return "string";
    }
    if (c === "{") return "string";
    stream.next();
  }
  return "string";
}

// LineStream is the subset of CodeMirror's StringStream the tokenizer uses,
// over a whole text, so tokenize works without CodeMirror
class LineStream {
  constructor(string) {
    this.string = string;
    this.pos = 0;
    this.start = 0;
  }
  eof() {
    return this.pos >= this.string.length;
  }
  eol() {
    return this.pos >= this.string.length || this.string[this.pos] === "\n";
  }
  peek() {
    return this.string[this.pos];
  }
  next() {
    return this.string[this.pos++];
  }
  match(pattern) {
    if (typeof pattern === "string") {
      if (this.string.startsWith(pattern, this.pos)) {
        this.pos += pattern.length;
        return true;
      }
      return false;
    }
    const m = pattern.exec(this.string.slice(this.pos));
    if (m && m.index === 0) {
      this.pos += m[0].length;
      return m;
    }
    return null;
  }
}
