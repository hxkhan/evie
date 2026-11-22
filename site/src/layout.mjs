// The parts every page shares, stamped into each page by build-content.mjs so
// that the header, footer and head live in one place.

export const repoURL = "https://github.com/hxkhan/evie";

// the mark: a lowercase e drawn as one stroke inside a warm disc
export const mark = `<svg class="mark" viewBox="0 0 32 32" aria-hidden="true"><circle cx="16" cy="16" r="15"/><path d="M9.2 16.6h13.6a6.9 6.9 0 1 0-2.2 5.1"/></svg>`;

const themeScript = `<script>try{const t=localStorage.getItem("evie-theme");if(t)document.documentElement.dataset.theme=t}catch(e){}</script>`;

export function head({ title, description, base, entry }) {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${title}</title>
<meta name="description" content="${description}">
<link rel="icon" href="${base}favicon.svg" type="image/svg+xml">
${themeScript}
<script type="module" src="${entry}"></script>
</head>`;
}

export function header({ base, active }) {
  const link = (href, label, key) =>
    `<a href="${base}${href}"${active === key ? ' aria-current="page"' : ""}>${label}</a>`;
  return `<header class="site-header">
  <div class="site-header__inner">
    <a class="brand" href="${base}">${mark}<span>evie</span></a>
    <nav class="site-nav" aria-label="Main">
      ${link("docs/", "Docs", "docs")}
      ${link("play/", "Playground", "play")}
      <a href="${repoURL}" rel="noopener">GitHub</a>
    </nav>
    <button class="theme-toggle" type="button" aria-label="Switch between light and dark" title="Switch theme">
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3a9 9 0 1 0 9 9 7 7 0 0 1-9-9z"/></svg>
    </button>
  </div>
</header>`;
}

export function footer({ base }) {
  return `<footer class="site-footer">
  <div class="site-footer__inner">
    <a class="brand brand--small" href="${base}">${mark}<span>evie</span></a>
    <p>A dynamically typed scripting language in pure Go. Apache-2.0.</p>
    <nav aria-label="Footer">
      <a href="${base}docs/">Guide</a>
      <a href="${base}play/">Playground</a>
      <a href="${repoURL}" rel="noopener">Source</a>
    </nav>
  </div>
</footer>`;
}
