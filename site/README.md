# Evie's website

The guide and a playground that runs Evie in the browser. Everything is
generated from the repository, so the site never drifts from the code:

- the guide comes from [`docs/syntax.md`](../docs/syntax.md), one page per
  `##` section, and Getting Started from the README's setup section;
- the playground's examples are the programs in [`src/samples/`](src/samples),
  listed with their titles in [`scripts/build-content.mjs`](scripts/build-content.mjs);
- the playground runs Evie compiled to WebAssembly ([`wasm/main.go`](wasm/main.go))
  inside a Web Worker, so a busy program never freezes the page and Stop can
  end any program.

## Working on it

Needs Go and Node 20 or newer.

```bash
cd site
npm install
npm run wasm      # build public/evie.wasm (again after changing Evie itself)
npm run dev       # generate the pages and serve them with live reload
npm run build     # everything, into dist/
npm test          # run programs through evie.wasm under Node
```

`npm run dev` regenerates the pages once; after editing `docs/syntax.md`,
`src/samples/` or `src/pages`, run `npm run content` again.

With the site built and `npx vite preview --port 4173` running,
`node scripts/test-browser.mjs` drives headless Chrome through the playground:
loading, running, stdin, errors, stopping a runaway loop, and fitting a
phone's width. `SHOTS=<dir>` also saves screenshots.

## Layout

| Path | What it is |
|---|---|
| `scripts/build-content.mjs` | writes `index.html`, `play/`, `docs/` and `src/generated/` (all generated, not committed) |
| `scripts/build-wasm.mjs` | builds `public/evie.wasm` and copies Go's `wasm_exec.js` |
| `src/layout.mjs` | the head, header and footer every page shares |
| `src/pages/` | the bodies of the home page and the playground |
| `src/evie-syntax.js` | Evie's highlighting, used by the docs at build time and by the editor |
| `src/styles.css` | the whole design: tokens for light and dark, then each page |
| `src/play.ts` | the playground: editor, worker, output, sharing |
| `public/evie-worker.js` | the worker that loads and runs Evie |

## Publishing

`.github/workflows/site.yml` builds the site on every change that can affect
it and deploys `main` to GitHub Pages. Turn Pages on once under the
repository's Settings → Pages, with **GitHub Actions** as the source. The
build uses `SITE_BASE=/<repository>/` for a project page; set it to `/` for a
custom domain.
