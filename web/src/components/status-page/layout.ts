import { arrayMove } from "@dnd-kit/sortable";
import type { StatusPageConfig, StatusPageMonitor, StatusSection } from "@/lib/api";

// Pure edits to the status page layout (sections and the monitors in them).
// A monitor is on the page when it's public, in its section. Monitors keep
// one global order; a section shows its monitors in that order.

type Layout = Pick<StatusPageConfig, "sections" | "monitors">;

export const MAX_SECTIONS = 20;

export const newSectionId = () => `s-${Math.random().toString(36).slice(2, 8)}`;

export function monitorsIn<T extends Layout>(layout: T, sectionId: string): StatusPageMonitor[] {
  return layout.monitors.filter((m) => m.public && m.section === sectionId);
}

/** Monitors not on the page, which can be added. */
export function notShown<T extends Layout>(layout: T): StatusPageMonitor[] {
  return layout.monitors.filter((m) => !m.public);
}

/** Puts monitors on the page, at the end of a section, in the order given. */
export function addMonitors<T extends Layout>(layout: T, ids: number[], sectionId: string): T {
  const adding = ids
    .map((id) => layout.monitors.find((m) => m.id === id))
    .filter((m): m is StatusPageMonitor => m !== undefined)
    .map((m) => ({ ...m, public: true, section: sectionId }));
  return { ...layout, monitors: [...layout.monitors.filter((m) => !ids.includes(m.id)), ...adding] };
}

/** Takes a monitor off the page. Its public name is kept for next time. */
export function removeMonitor<T extends Layout>(layout: T, id: number): T {
  return updateMonitor(layout, id, { public: false });
}

export function updateMonitor<T extends Layout>(layout: T, id: number, patch: Partial<StatusPageMonitor>): T {
  return { ...layout, monitors: layout.monitors.map((m) => (m.id === id ? { ...m, ...patch } : m)) };
}

/** Moves a monitor to where another one is (a drag within a section). */
export function reorderMonitors<T extends Layout>(layout: T, activeId: number, overId: number): T {
  const from = layout.monitors.findIndex((m) => m.id === activeId);
  const to = layout.monitors.findIndex((m) => m.id === overId);
  if (from < 0 || to < 0 || from === to) return layout;
  return { ...layout, monitors: arrayMove(layout.monitors, from, to) };
}

export function updateSection<T extends Layout>(layout: T, id: string, patch: Partial<StatusSection>): T {
  return { ...layout, sections: layout.sections.map((s) => (s.id === id ? { ...s, ...patch } : s)) };
}

/** Moves a section to where another one is (a drag). */
export function reorderSections<T extends Layout>(layout: T, activeId: string, overId: string): T {
  const from = layout.sections.findIndex((s) => s.id === activeId);
  const to = layout.sections.findIndex((s) => s.id === overId);
  if (from < 0 || to < 0 || from === to) return layout;
  return { ...layout, sections: arrayMove(layout.sections, from, to) };
}

export function addSection<T extends Layout>(layout: T, name = "New section", id = newSectionId()): T {
  if (layout.sections.length >= MAX_SECTIONS) return layout;
  return { ...layout, sections: [...layout.sections, { id, name }] };
}

/** Deletes a section; its monitors come off the page, as on the platform. */
export function removeSection<T extends Layout>(layout: T, id: string): T {
  return {
    ...layout,
    sections: layout.sections.filter((s) => s.id !== id),
    monitors: layout.monitors.map((m) => (m.section === id ? { ...m, public: false } : m)),
  };
}
