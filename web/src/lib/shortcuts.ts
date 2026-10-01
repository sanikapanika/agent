import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { toggleSidebarCollapsed } from "@/lib/sidebar";

// Keyboard shortcuts, matching upti.my-app hooks/useKeyboardShortcuts.ts
// where the agent has the same page: ⌘K / Ctrl K opens the command palette,
// "[" collapses the sidebar, "g …" goes to a page, "n …" creates something.

export const PALETTE_KEYS = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘K" : "Ctrl K";

// Second key of a "g …" / "n …" sequence → route. The command palette shows
// these; keep the two in sync.
const SEQUENCES: Record<string, Record<string, string>> = {
  g: {
    d: "/",
    h: "/healthchecks",
    b: "/heartbeats",
    s: "/status-page",
    m: "/maintenance",
    a: "/notifications",
    ",": "/settings",
  },
  n: {
    h: "/healthchecks/new",
    b: "/heartbeats/new",
  },
};

/** The shortcut for a route, e.g. "g h", for display. */
export function shortcutFor(path: string): string | undefined {
  for (const [prefix, keys] of Object.entries(SEQUENCES)) {
    for (const [key, target] of Object.entries(keys)) {
      if (target === path) return `${prefix} ${key}`;
    }
  }
}

function isTyping() {
  const el = document.activeElement as HTMLElement | null;
  const tag = el?.tagName.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select" || !!el?.isContentEditable;
}

/** Installs the shortcuts; returns the command palette's open state. */
export function useKeyboardShortcuts({ canEdit }: { canEdit: boolean }) {
  const navigate = useNavigate();
  const [paletteOpen, setPaletteOpen] = useState(false);
  const pending = useRef<{ key: string; at: number } | null>(null);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      // ⌘K works even while typing.
      if (e.key.toLowerCase() === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setPaletteOpen((open) => !open);
        return;
      }
      if (isTyping() || e.metaKey || e.ctrlKey || e.altKey) return;

      const prev = pending.current;
      pending.current = null;
      if (prev && Date.now() - prev.at < 1000) {
        const target = SEQUENCES[prev.key]?.[e.key.toLowerCase()];
        if (target && (prev.key !== "n" || canEdit)) {
          e.preventDefault();
          navigate(target);
        }
        return;
      }
      if (e.key === "[") {
        e.preventDefault();
        toggleSidebarCollapsed();
      } else if (e.key === "g" || e.key === "n") {
        pending.current = { key: e.key, at: Date.now() };
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [navigate, canEdit]);

  return { paletteOpen, openPalette: () => setPaletteOpen(true), closePalette: () => setPaletteOpen(false) };
}
