import { describe, expect, it } from "vitest";
import { formatActiveDuration, formatDuration, formatTimestamp } from "./format";

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

describe("formatActiveDuration", () => {
  it("distinguishes confirmed and estimated active durations", () => {
    expect(formatActiveDuration({ activeDurationSeconds: 2.4, dispatchedAt: "2026-07-18T01:00:00Z" })).toBe("2.40s");
    expect(formatActiveDuration(
      { activeDurationSeconds: null, dispatchedAt: "2026-07-18T01:00:00Z" },
      Date.parse("2026-07-18T01:00:03Z"),
    )).toBe("~3s");
    expect(formatActiveDuration({ activeDurationSeconds: null, dispatchedAt: null })).toBe("—");
  });
});
