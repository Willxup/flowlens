import type {
  LiveSampleResponse,
  LiveTargetsResponse,
  StatusResponse,
} from "../../api/contracts";

export interface LiveChartPoint {
  timestamp: number;
  upload: number | null;
  download: number | null;
}

export interface LiveView {
  currentUpload: number | null;
  currentDownload: number | null;
  averageUpload1m: number | null;
  averageDownload1m: number | null;
  averageUpload5m: number | null;
  averageDownload5m: number | null;
  peakUpload60m: number | null;
  peakDownload60m: number | null;
  activeConnections: number | null;
  connectionCoverage: number | null;
  status: StatusResponse;
  connected: boolean;
  observedAt: number | null;
  intervalMillis: number | null;
  targetGlobalRate: number | null;
  hasGap: boolean;
  chart: LiveChartPoint[];
  targets: LiveTargetsResponse["targets"];
}

export function buildLiveView(
  sourceSamples: LiveSampleResponse[],
  status: StatusResponse,
  targets: LiveTargetsResponse | null,
  connected: boolean,
): LiveView {
  const samples = [...sourceSamples]
    .filter(validSample)
    .sort((left, right) => left.timestamp - right.timestamp)
    .slice(-3600);
  const current = samples.at(-1);
  const chart = chartPoints(samples);
  return {
    currentUpload: current?.upload_bytes_per_second ?? null,
    currentDownload: current?.download_bytes_per_second ?? null,
    averageUpload1m: average(samples.slice(-60), "upload_bytes_per_second"),
    averageDownload1m: average(samples.slice(-60), "download_bytes_per_second"),
    averageUpload5m: average(samples.slice(-300), "upload_bytes_per_second"),
    averageDownload5m: average(
      samples.slice(-300),
      "download_bytes_per_second",
    ),
    peakUpload60m: peak(samples, "upload_bytes_per_second"),
    peakDownload60m: peak(samples, "download_bytes_per_second"),
    activeConnections:
      current?.active_connections ?? targets?.active_connections ?? null,
    connectionCoverage: targets?.connection_coverage ?? null,
    status,
    connected,
    observedAt: targets?.observed_at ?? null,
    intervalMillis: targets?.interval_millis ?? null,
    targetGlobalRate:
      targets === null
        ? null
        : targets.global_upload_bytes_per_second +
          targets.global_download_bytes_per_second,
    hasGap: chart.some((point) => point.upload === null),
    chart,
    targets: targets?.targets ?? [],
  };
}

function validSample(sample: LiveSampleResponse): boolean {
  return (
    Number.isSafeInteger(sample.timestamp) &&
    sample.timestamp > 0 &&
    sample.upload_bytes_per_second >= 0 &&
    sample.download_bytes_per_second >= 0 &&
    sample.active_connections >= 0
  );
}

function average(
  samples: LiveSampleResponse[],
  key: "upload_bytes_per_second" | "download_bytes_per_second",
): number | null {
  if (samples.length === 0) return null;
  return samples.reduce((sum, sample) => sum + sample[key], 0) / samples.length;
}

function peak(
  samples: LiveSampleResponse[],
  key: "upload_bytes_per_second" | "download_bytes_per_second",
): number | null {
  if (samples.length === 0) return null;
  return samples.reduce((value, sample) => Math.max(value, sample[key]), 0);
}

function chartPoints(samples: LiveSampleResponse[]): LiveChartPoint[] {
  const result: LiveChartPoint[] = [];
  let previous = 0;
  for (const sample of samples) {
    if (previous > 0 && sample.timestamp - previous > 2.5) {
      result.push({ timestamp: previous + 1, upload: null, download: null });
    }
    result.push({
      timestamp: sample.timestamp,
      upload: sample.upload_bytes_per_second,
      download: sample.download_bytes_per_second,
    });
    previous = sample.timestamp;
  }
  return result;
}
