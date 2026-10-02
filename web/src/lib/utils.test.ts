import { describe, expect, it } from "vitest";
import { formatSpan, nearestStep } from "./utils";

describe("formatSpan", () => {
  it("names the largest whole unit", () => {
    expect(formatSpan(60)).toBe("1 minute");
    expect(formatSpan(90)).toBe("90 seconds");
    expect(formatSpan(21600)).toBe("6 hours");
    expect(formatSpan(86400)).toBe("1 day");
    expect(formatSpan(604800)).toBe("1 week");
    expect(formatSpan(2592000)).toBe("30 days");
    expect(formatSpan(31536000)).toBe("1 year");
  });
});

describe("nearestStep", () => {
  it("places a typed value on the closest stop", () => {
    const steps = [60, 300, 3600, 86400];
    expect(nearestStep(300, steps)).toBe(1);
    expect(nearestStep(4000, steps)).toBe(2);
    expect(nearestStep(1, steps)).toBe(0);
    expect(nearestStep(10 ** 9, steps)).toBe(3);
  });
});
