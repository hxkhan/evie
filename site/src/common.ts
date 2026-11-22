// What every page loads: the fonts, the styles and the theme toggle.
import "@fontsource-variable/source-serif-4";
import "@fontsource-variable/dm-sans";
import "@fontsource-variable/jetbrains-mono";
import "./styles.css";

const root = document.documentElement;

function currentTheme(): "light" | "dark" {
  const set = root.dataset.theme;
  if (set === "light" || set === "dark") return set;
  return matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

document.querySelector<HTMLButtonElement>(".theme-toggle")?.addEventListener("click", () => {
  const next = currentTheme() === "dark" ? "light" : "dark";
  root.dataset.theme = next;
  try {
    localStorage.setItem("evie-theme", next);
  } catch {
    // storage can be unavailable; the choice then lasts for this page only
  }
});
