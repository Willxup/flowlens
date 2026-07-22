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
  uploadRate: number;
  downloadRate: number;
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
      uploadRate: point.average_upload_bytes_per_second,
      downloadRate: point.average_download_bytes_per_second,
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
    averageUpload: weightedAverage(series, "average_upload_bytes_per_second"),
    averageDownload: weightedAverage(
      series,
      "average_download_bytes_per_second",
    ),
    peakUpload: max(series, "peak_upload_bytes_per_second"),
    peakDownload: max(series, "peak_download_bytes_per_second"),
    averageConnections:
      connectionSamples > 0 ? connectionSum / connectionSamples : null,
    peakConnections: series.points.reduce(
      (value, point) => Math.max(value, point.active_connections_max),
      0,
    ),
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

function weightedAverage(
  series: SeriesResponse,
  key: "average_upload_bytes_per_second" | "average_download_bytes_per_second",
) {
  const elapsed = series.points.reduce(
    (sum, point) => sum + point.elapsed_seconds,
    0,
  );
  if (elapsed === 0) return null;
  return (
    series.points.reduce(
      (sum, point) => sum + point[key] * point.elapsed_seconds,
      0,
    ) / elapsed
  );
}

function max(
  series: SeriesResponse,
  key: "peak_upload_bytes_per_second" | "peak_download_bytes_per_second",
) {
  if (series.points.length === 0) return null;
  return series.points.reduce((value, point) => Math.max(value, point[key]), 0);
}
