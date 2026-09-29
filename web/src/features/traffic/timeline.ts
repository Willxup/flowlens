import type { SeriesResponse } from "../../api/contracts";
import type { LiveChartPoint } from "../live/model";

export type TimelinePoint = LiveChartPoint & { resolution: number };
export const DAY_SECONDS = 86400;

// Historical averages precede the available raw window. Never fabricate second
// samples from aggregates, or connect across missing collection intervals.
export function buildTimeline(
  history: SeriesResponse | null,
  live: LiveChartPoint[],
  now: number,
) {
  const from = now - DAY_SECONDS;
  const raw = live.filter(
    (point) => point.timestamp > now - 3600 && point.timestamp <= now,
  );
  const firstRaw =
    raw.find((point) => point.upload !== null || point.download !== null)
      ?.timestamp ?? Infinity;
  const archive: TimelinePoint[] = (history?.points ?? [])
    .filter(
      (point) =>
        point.bucket_end > from &&
        point.bucket_start <= now &&
        point.bucket_end <= firstRaw,
    )
    .map((point) => ({
      timestamp: Math.max(from, point.bucket_start),
      upload:
        point.speed_sample_count > 0
          ? point.average_upload_bytes_per_second
          : null,
      download:
        point.speed_sample_count > 0
          ? point.average_download_bytes_per_second
          : null,
      resolution: point.bucket_end - point.bucket_start,
    }));
  const ordered = [
    ...archive,
    ...raw.map((point) => ({ ...point, resolution: 1 })),
  ].sort((a, b) => a.timestamp - b.timestamp);
  const points: TimelinePoint[] = [];
  for (const point of ordered) {
    const previous = points.at(-1);
    if (
      previous &&
      previous.upload !== null &&
      previous.download !== null &&
      (point.timestamp - previous.timestamp >
        Math.max(previous.resolution, 2.5) ||
        previous.resolution !== point.resolution)
    ) {
      points.push({
        timestamp: Math.min(
          previous.timestamp + previous.resolution,
          point.timestamp - 0.001,
        ),
        upload: null,
        download: null,
        resolution: previous.resolution,
      });
    }
    points.push(point);
  }
  // The preview always spans the same real 24-hour axis; raw samples are reduced
  // to minute means for this small overview only, never for the detailed plot.
  const buckets = new Map<
    number,
    { upload: number; download: number; count: number }
  >();
  for (const point of ordered) {
    if (point.upload === null || point.download === null) continue;
    for (
      let minute = Math.floor(point.timestamp / 60) * 60;
      minute < Math.min(now + 1, point.timestamp + point.resolution);
      minute += 60
    ) {
      const value = buckets.get(minute) ?? { upload: 0, download: 0, count: 0 };
      value.upload += point.upload;
      value.download += point.download;
      value.count++;
      buckets.set(minute, value);
    }
  }
  const preview: LiveChartPoint[] = [];
  for (
    let timestamp = Math.ceil(from / 60) * 60;
    timestamp <= now;
    timestamp += 60
  ) {
    const value = buckets.get(timestamp);
    preview.push({
      timestamp,
      upload: value ? value.upload / value.count : null,
      download: value ? value.download / value.count : null,
    });
  }
  return { points, preview };
}

export function clampWindow(
  range: readonly [number, number],
  now: number,
): [number, number] {
  const end = Math.max(now - DAY_SECONDS + 60, Math.min(now, range[1]));
  return [Math.max(now - DAY_SECONDS, Math.min(end - 60, range[0])), end];
}

export function precisionLabel(points: TimelinePoint[]): string {
  const present = points.filter(
    (point) => point.upload !== null || point.download !== null,
  );
  const raw = present.some((point) => point.resolution === 1);
  const aggregate = present.some((point) => point.resolution > 1);
  return raw && aggregate
    ? "秒级样本 + 历史聚合"
    : raw
      ? "秒级样本"
      : aggregate
        ? "历史聚合 · 非秒级"
        : "暂无采样数据";
}
