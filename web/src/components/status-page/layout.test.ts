import { describe, expect, it } from "vitest";
import type { StatusPageMonitor } from "@/lib/api";
import {
  addMonitors,
  addSection,
  MAX_SECTIONS,
  monitorsIn,
  notShown,
  removeMonitor,
  removeSection,
  reorderMonitors,
  reorderSections,
} from "./layout";

const mon = (id: number, section: string, shown = true): StatusPageMonitor => ({
  id,
  name: `m${id}`,
  kind: "healthcheck",
  type_label: "HTTP",
  source: "ui",
  public: shown,
  label: "",
  section,
});

const layout = {
  sections: [
    { id: "a", name: "A" },
    { id: "b", name: "B" },
  ],
  monitors: [mon(1, "a"), mon(2, "b"), mon(3, "a"), mon(4, "b"), mon(5, "a", false)],
};

const ids = (l: typeof layout, section: string) => monitorsIn(l, section).map((m) => m.id);

describe("status page layout", () => {
  it("shows only the monitors on the page, by section", () => {
    expect(ids(layout, "a")).toEqual([1, 3]);
    expect(notShown(layout).map((m) => m.id)).toEqual([5]);
  });

  it("reorders monitors within a section by dragging", () => {
    expect(ids(reorderMonitors(layout, 3, 1), "a")).toEqual([3, 1]);
    expect(ids(reorderMonitors(layout, 3, 1), "b")).toEqual([2, 4]);
    expect(reorderMonitors(layout, 3, 3)).toBe(layout);
  });

  it("reorders sections by dragging", () => {
    expect(reorderSections(layout, "b", "a").sections.map((s) => s.id)).toEqual(["b", "a"]);
  });

  it("adds monitors to the end of a section and takes them off again", () => {
    const added = addMonitors(layout, [5], "b");
    expect(ids(added, "b")).toEqual([2, 4, 5]);
    expect(notShown(added)).toEqual([]);
    expect(ids(removeMonitor(added, 4), "b")).toEqual([2, 5]);
  });

  it("takes a deleted section's monitors off the page", () => {
    const removed = removeSection(layout, "a");
    expect(removed.sections.map((s) => s.id)).toEqual(["b"]);
    expect(notShown(removed).map((m) => m.id)).toEqual([1, 3, 5]);
    expect(removeSection(removed, "b").sections).toEqual([]);
  });

  it("names new sections and caps how many there are", () => {
    expect(addSection(layout, undefined, "c").sections.at(-1)).toEqual({ id: "c", name: "New section" });
    let l = layout;
    for (let i = 0; i < 30; i++) l = addSection(l, "x", `s${i}`);
    expect(l.sections).toHaveLength(MAX_SECTIONS);
  });
});
