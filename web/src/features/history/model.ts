import type {
  ByteString,
  OverviewResponse,
  QualityResponse,
  SeriesResponse,
} from "../../api/contracts";
import { addByteStrings } from "../../lib/format";

export interface HistoricalChartPoint {
  start: number;
  end: number;
  upload: ByteString;
  download: ByteString;
  cumulative: ByteString;
  uploadRate: number | null;
  downloadRate: number | null;
  resolution: number;
}

export function buildHistoricalView(
  overview: OverviewResponse,
  series: SeriesResponse,
  quality: QualityResponse,
) {
  let cumulative = "0" as ByteString;
  const chart = series.points.map((point) => {
    cumulative = addByteStrings(
      cumulative,
      point.upload_bytes,
      point.download_bytes,
    );
    return {
      start: point.bucket_start,
      end: point.bucket_end,
      upload: point.upload_bytes,
      download: point.download_bytes,
      cumulative,
      uploadRate:
        point.speed_sample_count > 0
          ? point.average_upload_bytes_per_second
          : null,
      downloadRate:
        point.speed_sample_count > 0
          ? point.average_download_bytes_per_second
          : null,
      resolution: point.source_resolution_sec,
    } satisfies HistoricalChartPoint;
  });
  const observed = series.points.reduce(
    (sum, point) => sum + point.counter_observed_seconds,
    0,
  );
  const elapsed = series.points.reduce(
    (sum, point) => sum + point.elapsed_seconds,
    0,
  );
  const connectionSamples = series.points.reduce(
    (sum, point) => sum + point.active_connections_samples,
    0,
  );
  const connectionSum = series.points.reduce(
    (sum, point) => sum + point.active_connections_sum,
    0,
  );
  const recoveredBytes = series.points.reduce(
    (sum, point) =>
      addByteStrings(
        sum,
        point.recovered_upload_bytes,
        point.recovered_download_bytes,
      ),
    "0" as ByteString,
  );
  const unattributedBytes = series.points.reduce(
    (sum, point) =>
      addByteStrings(
        sum,
        point.unattributed_upload_bytes,
        point.unattributed_download_bytes,
      ),
    "0" as ByteString,
  );
  return {
    uploadBytes: overview.current.upload_bytes,
    downloadBytes: overview.current.download_bytes,
    totalBytes: addByteStrings(
      overview.current.upload_bytes,
      overview.current.download_bytes,
    ),
    previousBytes: overview.previous.total_bytes,
    completeness: elapsed > 0 ? Math.min(1, observed / elapsed) : null,
    averageUpload: sampledAverage(series, "speed_upload_sample_sum"),
    averageDownload: sampledAverage(series, "speed_download_sample_sum"),
    peakUpload: max(series, "peak_upload_bytes_per_second"),
    peakDownload: max(series, "peak_download_bytes_per_second"),
    averageConnections:
      connectionSamples > 0 ? connectionSum / connectionSamples : null,
    peakConnections:
      connectionSamples > 0
        ? series.points.reduce(
            (value, point) =>
              point.active_connections_samples > 0
                ? Math.max(value, point.active_connections_max)
                : value,
            0,
          )
        : null,
    boundaryApproximate:
      overview.boundary_approximate || series.boundary_approximate,
    qualityEvents: quality.events,
    recoveredBytes,
    unattributedBytes,
    resetCount: series.points.reduce(
      (sum, point) => sum + point.reset_count,
      0,
    ),
    chart,
  };
}

function sampledAverage(
  series: SeriesResponse,
  key: "speed_upload_sample_sum" | "speed_download_sample_sum",
) {
  const samples = series.points.reduce(
    (sum, point) => sum + point.speed_sample_count,
    0,
  );
  if (samples === 0) return null;
  const sum = series.points.reduce(
    (value, point) => value + BigInt(point[key]),
    0n,
  );
  const count = BigInt(samples);
  return Number(sum / count) + Number(sum % count) / samples;
}

function max(
  series: SeriesResponse,
  key: "peak_upload_bytes_per_second" | "peak_download_bytes_per_second",
) {
  const sampled = series.points.filter((point) => point.speed_sample_count > 0);
  if (sampled.length === 0) return null;
  return sampled.reduce((value, point) => Math.max(value, point[key]), 0);
}
