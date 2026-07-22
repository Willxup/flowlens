import type {
  OverviewResponse,
  QualityResponse,
  SeriesResponse,
} from "../../../api/contracts";
import { asByteString } from "../../../lib/format";
import { buildHistoricalView } from "../model";

const totals = (
  upload: string,
  download: string,
  elapsed = 100,
  observed = 90,
) => ({
  upload_bytes: asByteString(upload),
  download_bytes: asByteString(download),
  total_bytes: asByteString((BigInt(upload) + BigInt(download)).toString()),
  elapsed_seconds: elapsed,
  observed_seconds: observed,
});

describe("buildHistoricalView", () => {
  it("keeps exact totals and derives only sampled metrics from series", () => {
    const overview: OverviewResponse = {
      current: totals("9007199254740993", "7"),
      previous: totals("10", "20"),
      boundary_approximate: false,
    };
    const series: SeriesResponse = {
      boundary_approximate: false,
      points: [
        {
          bucket_start: 10,
          bucket_end: 20,
          elapsed_seconds: 10,
          source_resolution_sec: 10,
          upload_bytes: asByteString("5"),
          download_bytes: asByteString("7"),
          recovered_upload_bytes: asByteString("2"),
          recovered_download_bytes: asByteString("3"),
          unattributed_upload_bytes: asByteString("1"),
          unattributed_download_bytes: asByteString("2"),
          average_upload_bytes_per_second: 3,
          average_download_bytes_per_second: 4,
          peak_upload_bytes_per_second: 8,
          peak_download_bytes_per_second: 9,
          counter_observed_seconds: 9,
          active_connections_sum: 30,
          active_connections_samples: 10,
          active_connections_max: 5,
          reset_count: 2,
          quality_flags: 8,
        },
      ],
    };
    const quality: QualityResponse = {
      events: [{ code: "gap", started_at: 10, ended_at: 12, flags: 8 }],
    };
    const view = buildHistoricalView(overview, series, quality);
    expect(view.totalBytes).toBe("9007199254741000");
    expect(view.completeness).toBe(0.9);
    expect(view.averageConnections).toBe(3);
    expect(view.peakDownload).toBe(9);
    expect(view.chart[0]?.cumulative).toBe("12");
    expect(view.recoveredBytes).toBe("5");
    expect(view.unattributedBytes).toBe("3");
    expect(view.resetCount).toBe(2);
    expect(view.qualityEvents).toHaveLength(1);
  });
});
