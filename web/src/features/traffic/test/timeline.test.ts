import type {
  SeriesPointResponse,
  SeriesResponse,
} from "../../../api/contracts";
import { buildTimeline, clampWindow, precisionLabel } from "../timeline";
const now = 200000;
function series(
  start: number,
  end: number,
  rate: number,
  count = 60,
): SeriesResponse {
  return {
    boundary_approximate: false,
    points: [
      {
        bucket_start: start,
        bucket_end: end,
        speed_sample_count: count,
        average_upload_bytes_per_second: rate,
        average_download_bytes_per_second: rate * 2,
      } as SeriesPointResponse,
    ],
  };
}
describe("24-hour timeline", () => {
  it("uses archived averages before available raw samples without replacing second detail", () => {
    const history = series(now - 7200, now - 7140, 4);
    const live = [
      { timestamp: now - 1, upload: 10, download: 20 },
      { timestamp: now, upload: 20, download: 40 },
    ];
    const result = buildTimeline(history, live, now);
    expect(result.points[0]).toMatchObject({
      timestamp: now - 7200,
      resolution: 60,
      upload: 4,
    });
    expect(result.points.at(-1)).toMatchObject({
      timestamp: now,
      resolution: 1,
      upload: 20,
    });
    expect(result.points.some((p) => p.upload === null)).toBe(true);
    expect(precisionLabel(result.points)).toBe("秒级样本 + 历史聚合");
    expect(result.preview).toHaveLength(1440);
  });
  it("uses durable aggregates after a restart and does not invent second samples", () => {
    const result = buildTimeline(series(now - 120, now - 60, 0), [], now);
    expect(result.points).toHaveLength(1);
    expect(result.points[0]).toMatchObject({
      upload: 0,
      download: 0,
      resolution: 60,
    });
    expect(precisionLabel(result.points)).toBe("历史聚合 · 非秒级");
    expect(result.preview.filter((p) => p.upload === 0)).toHaveLength(2);
    expect(
      buildTimeline(series(now - 120, now - 60, 0, 0), [], now).points[0]
        ?.upload,
    ).toBeNull();
  });
  it("does not overlap archive buckets with the raw window or bridge raw gaps", () => {
    const result = buildTimeline(
      series(now - 60, now, 999),
      [
        { timestamp: now - 20, upload: 1, download: 2 },
        { timestamp: now - 19, upload: null, download: null },
        { timestamp: now, upload: 3, download: 4 },
      ],
      now,
    );
    expect(result.points.some((p) => p.upload === 999)).toBe(false);
    expect(result.points[1]?.upload).toBeNull();
  });
  it("drops samples outside the retained window and bounds selection without changing its absolute time", () => {
    expect(
      buildTimeline(
        null,
        [{ timestamp: now - 4000, upload: 5, download: 8 }],
        now,
      ).points,
    ).toEqual([]);
    expect(clampWindow([now - 7200, now - 3600], now + 300)).toEqual([
      now - 7200,
      now - 3600,
    ]);
    expect(clampWindow([0, now + 500], now)).toEqual([now - 86400, now]);
  });
});
