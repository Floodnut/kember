import { describe, expect, it } from "vitest";
import { formatDuration, formatTimestamp } from "./format";

describe("formatDuration", () => {
  it("formats null, milliseconds, and seconds", () => {
    expect(formatDuration(null)).toBe("—");
    expect(formatDuration(0.041)).toBe("41ms");
    expect(formatDuration(2.377)).toBe("2.38s");
  });
});

describe("formatTimestamp", () => {
  it("preserves a valid timestamp and replaces missing values", () => {
    expect(formatTimestamp(null)).toBe("—");
    expect(formatTimestamp("2026-07-18T01:00:00Z")).toBe("2026-07-18 01:00:00 UTC");
  });
});
