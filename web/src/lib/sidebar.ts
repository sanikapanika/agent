import { useSyncExternalStore } from "react";

// Mirrors upti.my-app hooks/useSidebarCollapsed.ts. Desktop only: the mobile
// drawer is always expanded. Stored per browser so it survives reloads.
//
// The width lives in a CSS variable, not React state, so the page offset
// follows a toggle without re-rendering the page: var(--sidebar-w, 15rem).

const STORAGE_KEY = "sidebarCollapsed";
const WIDTH = { expanded: "15rem", collapsed: "4rem" };

function read(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

let collapsed = read();
const listeners = new Set<() => void>();

function applyWidth() {
  document.documentElement.style.setProperty("--sidebar-w", collapsed ? WIDTH.collapsed : WIDTH.expanded);
}
applyWidth();

export function toggleSidebarCollapsed() {
  collapsed = !collapsed;
  applyWidth();
  try {
    localStorage.setItem(STORAGE_KEY, String(collapsed));
  } catch {
    // Storage blocked: the toggle still works for this page load.
  }
  listeners.forEach((l) => l());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Only the sidebar needs this; everything else uses the CSS variable. */
export const useSidebarCollapsed = () => useSyncExternalStore(subscribe, () => collapsed);
