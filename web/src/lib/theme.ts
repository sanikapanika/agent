import { useSyncExternalStore } from "react";

// Light, dark or follow the OS. The choice is per browser (localStorage); the
// `dark` class on <html> drives the CSS tokens and Tailwind's dark: variant.
// index.html applies the stored choice before first paint, so there's no
// flash of the wrong theme; keep its inline script in sync with this file.

export type ThemePreference = "light" | "dark" | "system";

const STORAGE_KEY = "uptimy-theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");
const listeners = new Set<() => void>();

function read(): ThemePreference {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === "light" || v === "dark") return v;
  } catch {
    // Storage blocked (private mode, sandboxed preview): follow the OS.
  }
  return "system";
}

let preference = read();

function apply() {
  const dark = preference === "dark" || (preference === "system" && media.matches);
  document.documentElement.classList.toggle("dark", dark);
  document.documentElement.style.colorScheme = dark ? "dark" : "light";
}

media.addEventListener("change", () => {
  if (preference === "system") {
    apply();
    listeners.forEach((l) => l());
  }
});

function setTheme(next: ThemePreference) {
  preference = next;
  try {
    if (next === "system") localStorage.removeItem(STORAGE_KEY);
    else localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // Not persisted; still applies for this page view.
  }
  apply();
  listeners.forEach((l) => l());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** The stored preference and what it resolves to right now. */
export function useTheme() {
  const pref = useSyncExternalStore(subscribe, () => preference);
  const resolved = useSyncExternalStore(subscribe, () =>
    preference === "system" ? (media.matches ? "dark" : "light") : preference,
  );
  return { preference: pref, resolved, setTheme };
}

apply();
