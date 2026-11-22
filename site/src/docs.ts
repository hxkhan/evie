// Guide pages: copy buttons on code blocks and an "On this page" list that
// follows the reader.
import "./common";

for (const button of document.querySelectorAll<HTMLButtonElement>(".code-copy")) {
  button.addEventListener("click", async () => {
    const code = button.parentElement?.querySelector("pre")?.textContent ?? "";
    try {
      await navigator.clipboard.writeText(code);
      button.textContent = "Copied";
    } catch {
      // no clipboard access: select the code so the reader can copy it
      const pre = button.parentElement?.querySelector("pre");
      if (pre) getSelection()?.selectAllChildren(pre);
      button.textContent = "Selected";
    }
    setTimeout(() => (button.textContent = "Copy"), 1500);
  });
}

const links = new Map<string, HTMLAnchorElement>();
for (const a of document.querySelectorAll<HTMLAnchorElement>(".toc a")) {
  links.set(decodeURIComponent(a.hash.slice(1)), a);
}
if (links.size > 0) {
  const headings = [...links.keys()].map((id) => document.getElementById(id)).filter((h): h is HTMLElement => h !== null);
  const observer = new IntersectionObserver(
    () => {
      // the active heading is the last one above the top third of the screen
      let active = headings[0];
      for (const h of headings) {
        if (h.getBoundingClientRect().top < innerHeight / 3) active = h;
      }
      for (const [id, a] of links) a.classList.toggle("is-active", id === active?.id);
    },
    { rootMargin: "0px 0px -60% 0px", threshold: [0, 1] },
  );
  for (const h of headings) observer.observe(h);
}
