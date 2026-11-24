// Generates the site's pages from the repository: the guide from
// docs/syntax.md, Getting Started from the README, the playground's examples
// from src/samples, and the home and playground pages from src/pages, each
// with the shared header and footer. Run it before vite; it writes
// index.html, play/, docs/ and src/generated/, which are not committed.
//
//	node scripts/build-content.mjs        (SITE_BASE=/evie/ for a project page)
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { Marked } from "marked";
import { tokenize } from "../src/evie-syntax.js";
import { footer, head, header, mark, repoURL } from "../src/layout.mjs";

const site = fileURLToPath(new URL("..", import.meta.url));
const repo = join(site, "..");
const base = process.env.SITE_BASE ?? "/";

const read = (p) => readFileSync(join(repo, p), "utf8").replaceAll("\r\n", "\n");
const write = (p, s) => {
  mkdirSync(dirname(join(site, p)), { recursive: true });
  writeFileSync(join(site, p), s);
};

// ---------------------------------------------------------------- helpers

const escapeHTML = (s) => s.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");

export function highlight(code, lang = "evie") {
  return tokenize(code, lang)
    .map(([text, type]) => (type ? `<span class="t-${type}">${escapeHTML(text)}</span>` : escapeHTML(text)))
    .join("");
}

const slugify = (s) =>
  s
    .replace(/<[^>]+>/g, "")
    .replace(/`/g, "")
    .replace(/^\d+\.\s*/, "")
    .toLowerCase()
    .replace(/&amp;|&/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");

// Evie code can open in the playground, which runs a program with a package
// main header as it is and anything else as a session; Go host code cannot
const isGoCode = (code) => /^\s*import\s*\(|\bfunc\s|\bvm\.[A-Z]|\bevie\.[A-Z]/m.test(code);

const playLink = (code) => `${base}play/#code=${Buffer.from(code, "utf8").toString("base64url")}`;

// links in the markdown point at files in the repository; on the site they go to GitHub
function resolveLink(href, from) {
  if (/^(https?:|mailto:|#)/.test(href)) return href;
  const path = new URL(href, `https://x/${from}`).pathname.slice(1);
  return `${repoURL}/blob/main/${path}`;
}

function markdown(from) {
  const md = new Marked();
  md.use({
    renderer: {
      code({ text, lang }) {
        const evie = !lang || lang === "go" || lang === "evie";
        const goHost = evie && isGoCode(text);
        const body = evie ? highlight(text, goHost ? "go" : "evie") : escapeHTML(text);
        const run = evie && !goHost ? `<a class="code-run" href="${playLink(text)}">Open in playground</a>` : "";
        return `<div class="code-block"><pre class="code"><code>${body}</code></pre>${run}<button class="code-copy" type="button" aria-label="Copy code">Copy</button></div>\n`;
      },
      heading({ tokens, depth, text }) {
        const html = this.parser.parseInline(tokens);
        const id = slugify(text);
        if (depth === 1) return "";
        return `<h${depth} id="${id}"><a class="anchor" href="#${id}" aria-hidden="true">#</a>${html}</h${depth}>\n`;
      },
      link({ href, title, tokens }) {
        const t = title ? ` title="${escapeHTML(title)}"` : "";
        const target = resolveLink(href, from);
        const ext = target.startsWith("http") && !target.startsWith(base) ? ' rel="noopener"' : "";
        return `<a href="${target}"${t}${ext}>${this.parser.parseInline(tokens)}</a>`;
      },
    },
  });
  // tables scroll on their own on narrow screens
  return {
    parse: (text) => md.parse(text).replaceAll("<table>", '<div class="table-wrap"><table>').replaceAll("</table>", "</table></div>"),
  };
}

const pageHTML = ({ title, description, active, entry, body }) =>
  `${head({ title, description, base, entry: `/src/${entry}` })}
<body class="page page--${active || "home"}">
${header({ base, active })}
${body}
${active === "play" ? "" : footer({ base })}
</body>
</html>
`;

// ---------------------------------------------------------------- the guide

const guide = read("docs/syntax.md");
const parts = guide.split(/^## /m);
const intro = parts[0].replace(/^# .*\n/, "").replace(/\n---\s*$/, "").trim();
const sections = parts.slice(1).map((chunk, i) => {
  const nl = chunk.indexOf("\n");
  const rawTitle = chunk.slice(0, nl).trim();
  const title = rawTitle.replace(/^\d+\.\s*/, "");
  const md = chunk.slice(nl + 1).replace(/\n---\s*$/, "").trim();
  const subs = [...md.matchAll(/^### (.+)$/gm)].map((m) => ({ title: m[1], id: slugify(m[1]) }));
  return { number: i + 1, rawTitle, title, slug: slugify(rawTitle), md, subs };
});

// Getting Started comes from the README's setup section
const readme = read("README.md");
const setup = (readme.match(/^## Setup & Quick Start\n([\s\S]*?)\n---/m) ?? [, ""])[1].trim();

const inline = (s) => new Marked().parseInline(s);

function docsNav(current) {
  const item = (href, label, key, subs) => {
    const here = key === current;
    const children =
      here && subs?.length
        ? `<ul>${subs.map((s) => `<li><a href="#${s.id}">${inline(s.title)}</a></li>`).join("")}</ul>`
        : "";
    return `<li${here ? ' class="is-current"' : ""}><a href="${base}${href}"${here ? ' aria-current="page"' : ""}>${label}</a>${children}</li>`;
  };
  return `<nav class="docs-nav__list" aria-label="Guide">
  <p class="docs-nav__heading">Start</p>
  <ul>${item("docs/", "Overview &amp; setup", "overview")}</ul>
  <p class="docs-nav__heading">Language guide</p>
  <ol>${sections.map((s) => item(`docs/${s.slug}/`, inline(s.title), s.slug, s.subs)).join("")}</ol>
</nav>`;
}

function docsPage({ key, title, eyebrow, html, subs, prev, next, source }) {
  const toc = subs?.length
    ? `<aside class="toc" aria-label="On this page"><p class="toc__heading">On this page</p><ul>${subs
        .map((s) => `<li><a href="#${s.id}">${inline(s.title)}</a></li>`)
        .join("")}</ul></aside>`
    : `<aside class="toc" aria-hidden="true"></aside>`;
  const pager = `<nav class="pager" aria-label="Pages">
  ${prev ? `<a class="pager__prev" href="${base}${prev.href}"><span>Previous</span>${prev.label}</a>` : "<span></span>"}
  ${next ? `<a class="pager__next" href="${base}${next.href}"><span>Next</span>${next.label}</a>` : "<span></span>"}
</nav>`;
  const body = `<div class="docs">
  <aside class="docs-nav">
    <details class="docs-nav__mobile"><summary>Contents</summary>${docsNav(key)}</details>
    <div class="docs-nav__desktop">${docsNav(key)}</div>
  </aside>
  <article class="prose">
    <p class="eyebrow">${eyebrow}</p>
    <h1>${title}</h1>
    ${html}
    <p class="edit"><a href="${repoURL}/blob/main/${source}" rel="noopener">Edit this page on GitHub</a></p>
    ${pager}
  </article>
  ${toc}
</div>`;
  return pageHTML({
    title: `${title.replace(/<[^>]+>/g, "")} · Evie`,
    description: "The Evie language guide",
    active: "docs",
    entry: "docs.ts",
    body,
  });
}

const md = markdown("docs/syntax.md");
const overviewHTML =
  md.parse(intro) +
  `<h2 id="getting-started"><a class="anchor" href="#getting-started" aria-hidden="true">#</a>Getting started</h2>\n` +
  markdown("README.md").parse(setup.replace(/^### /gm, "### "));
const overviewSubs = [{ title: "Getting started", id: "getting-started" }, ...[...setup.matchAll(/^### (.+)$/gm)].map((m) => ({ title: m[1], id: slugify(m[1]) }))];

rmSync(join(site, "docs"), { recursive: true, force: true });
write(
  "docs/index.html",
  docsPage({
    key: "overview",
    title: "The Evie guide",
    eyebrow: "Overview",
    html: overviewHTML,
    subs: overviewSubs,
    next: { href: `docs/${sections[0].slug}/`, label: inline(sections[0].title) },
    source: "docs/syntax.md",
  }),
);
sections.forEach((s, i) => {
  const prev = i === 0 ? { href: "docs/", label: "Overview &amp; setup" } : { href: `docs/${sections[i - 1].slug}/`, label: inline(sections[i - 1].title) };
  const next = i + 1 < sections.length ? { href: `docs/${sections[i + 1].slug}/`, label: inline(sections[i + 1].title) } : null;
  write(
    `docs/${s.slug}/index.html`,
    docsPage({
      key: s.slug,
      title: inline(s.title),
      eyebrow: `Language guide · ${s.number} of ${sections.length}`,
      html: md.parse(s.md),
      subs: s.subs,
      prev,
      next,
      source: "docs/syntax.md",
    }),
  );
});

// ---------------------------------------------------------------- examples

// the playground's examples, in the order the picker lists them; each is
// src/samples/<id>.ev
const samples = [
  { id: "hero", title: "Bank account" },
  { id: "words", title: "Word count" },
  { id: "shapes", title: "Shapes and inheritance" },
  { id: "closures", title: "Closures" },
  { id: "errors", title: "Handling errors" },
  { id: "parallel", title: "Waiting in parallel" },
  { id: "shared", title: "Shared state without locks" },
  { id: "temperatures", title: "Reading input" },
  { id: "fib", title: "Fibonacci" },
];
const examples = samples.map((s) => ({ ...s, code: readFileSync(join(site, "src/samples", `${s.id}.ev`), "utf8").replaceAll("\r\n", "\n") }));
write("src/generated/examples.json", JSON.stringify(examples, null, 2));

// ---------------------------------------------------------------- pages

const sample = (name) =>
  highlight(readFileSync(join(site, "src/samples", name), "utf8").replaceAll("\r\n", "\n").trimEnd(), name.endsWith(".go.txt") ? "go" : "evie");
const fill = (html) =>
  html
    .replaceAll("{{base}}", base)
    .replaceAll("{{repo}}", repoURL)
    .replace(/<pre class="code" data-sample="([^"]+)"><\/pre>/g, (_, name) => `<pre class="code"><code>${sample(name)}</code></pre>`);

write(
  "index.html",
  pageHTML({
    title: "Evie: a small, fast scripting language for Go",
    description: "Evie is a dynamically typed scripting language in pure Go, with immutable bindings, awaitable tasks, classes and easy embedding.",
    active: "",
    entry: "main.ts",
    body: fill(readFileSync(join(site, "src/pages/home.html"), "utf8")),
  }),
);
write(
  "play/index.html",
  pageHTML({
    title: "Playground · Evie",
    description: "Write and run Evie code in your browser.",
    active: "play",
    entry: "play.ts",
    body: fill(readFileSync(join(site, "src/pages/play.html"), "utf8")),
  }),
);

write("public/favicon.svg", mark.replace('class="mark"', 'xmlns="http://www.w3.org/2000/svg"').replace("<circle", '<circle fill="#c96442"').replace("<path", '<path fill="none" stroke="#fff" stroke-width="2.6" stroke-linecap="round"'));

console.log(`wrote the home page, the playground, ${sections.length + 1} guide pages and ${examples.length} examples (base ${base})`);
