import type {
  LiveSampleResponse,
  LiveTargetsResponse,
  StatusResponse,
} from "../../../api/contracts";
import { buildLiveView } from "../model";

const status: StatusResponse = {
  status: "ok",
  reason: "ready",
  timezone: "UTC",
  version: "v0.2.5",
  auth_enabled: true,
  capabilities: {
    connection_id: true,
    source: true,
    destination: true,
    port: true,
    network: true,
    domain: true,
  },
};
const targets: LiveTargetsResponse = {
  observed_at: 100,
  interval_millis: 1000,
  active_connections: 3,
  global_upload_bytes_per_second: 100,
  global_download_bytes_per_second: 200,
  connection_coverage: 0.9,
  targets: [],
};

function sample(
  timestamp: number,
  up: number,
  down: number,
): LiveSampleResponse {
  return {
    timestamp,
    upload_bytes_per_second: up,
    download_bytes_per_second: down,
    active_connections: 3,
    status: "ok",
  };
}

describe("buildLiveView", () => {
  it("computes current, moving averages, peaks and gaps from realtime samples only", () => {
    const samples = Array.from({ length: 301 }, (_, index) =>
      sample(index + 1, index + 1, (index + 1) * 2),
    );
    samples.push(sample(305, 500, 1000));
    const view = buildLiveView(samples, status, targets, true);
    expect(view.currentUpload).toBe(500);
    expect(view.averageUpload1m).toBeCloseTo(
      (500 + ((246 + 301) * 56) / 2) / 57,
    );
    expect(view.peakDownload60m).toBe(1000);
    expect(view.chart.some((point) => point.upload === null)).toBe(true);
    expect(view.connected).toBe(true);
    expect(view.observedAt).toBe(100);
    expect(view.intervalMillis).toBe(1000);
    expect(view.targetGlobalRate).toBe(300);
    expect(view.hasGap).toBe(true);
  });

  it("has an explicit empty state", () => {
    const view = buildLiveView([], status, null, false);
    expect(view.currentUpload).toBeNull();
    expect(view.activeConnections).toBeNull();
    expect(view.chart).toEqual([]);
  });

  it("excludes samples older than the actual 60 minute window", () => {
    const view = buildLiveView(
      [sample(1, 9999, 9999), sample(3602, 20, 10)],
      status,
      null,
      true,
    );
    expect(view.peakUpload60m).toBe(20);
    expect(view.chart).toHaveLength(1);
    expect(view.averageUpload1m).toBe(20);
    expect(view.sampledAt).toBe(3602);
  });
});
