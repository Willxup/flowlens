import type { TimeSelection } from "../../api/contracts";
import { toHistoricalRange } from "../time-range";

const at = (iso: string) => new Date(iso);

function range(selection: TimeSelection, now: string, timezone: string) {
  return toHistoricalRange(selection, at(now), timezone);
}

describe("historical ranges", () => {
  it("uses configured-zone natural day boundaries", () => {
    expect(
      range(
        { kind: "preset", preset: "today" },
        "2026-07-20T05:00:00Z",
        "Asia/Shanghai",
      ),
    ).toEqual({
      from: 1784476800,
      to: 1784523600,
    });
    expect(
      range(
        { kind: "preset", preset: "yesterday" },
        "2026-07-20T05:00:00Z",
        "Asia/Kathmandu",
      ),
    ).toEqual({
      from: 1784398500,
      to: 1784484900,
    });
  });

  it("preserves 23-hour and 25-hour New York days", () => {
    const spring = range(
      { kind: "preset", preset: "yesterday" },
      "2026-03-09T16:00:00Z",
      "America/New_York",
    );
    const fall = range(
      { kind: "preset", preset: "yesterday" },
      "2026-11-02T17:00:00Z",
      "America/New_York",
    );
    expect(spring.to - spring.from).toBe(23 * 3600);
    expect(fall.to - fall.from).toBe(25 * 3600);
  });

  it("supports rolling, year, lifetime and inclusive custom dates", () => {
    const now = "2026-07-20T05:00:00Z";
    expect(range({ kind: "preset", preset: "7d" }, now, "UTC")).toEqual({
      from: 1783918800,
      to: 1784523600,
    });
    expect(range({ kind: "preset", preset: "year" }, now, "UTC")).toEqual({
      from: 1767225600,
      to: 1784523600,
    });
    expect(range({ kind: "preset", preset: "lifetime" }, now, "UTC")).toEqual({
      from: 86400,
      to: 1784523600,
    });
    const custom = range(
      { kind: "custom", from: "2026-07-01", to: "2026-07-14" },
      now,
      "Asia/Shanghai",
    );
    expect(custom.to - custom.from).toBe(14 * 86400);
  });

  it("rejects live, invalid and reversed custom ranges", () => {
    expect(() =>
      range({ kind: "live" }, "2026-07-20T05:00:00Z", "UTC"),
    ).toThrow("not historical");
    expect(() =>
      range(
        { kind: "custom", from: "2026-02-30", to: "2026-03-01" },
        "2026-07-20T05:00:00Z",
        "UTC",
      ),
    ).toThrow("invalid custom range");
    expect(() =>
      range(
        { kind: "custom", from: "2026-07-20", to: "2026-07-19" },
        "2026-07-20T05:00:00Z",
        "UTC",
      ),
    ).toThrow("invalid custom range");
  });
});
