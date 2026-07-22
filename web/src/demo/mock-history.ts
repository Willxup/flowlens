import fixtureJSON from "./fixture.json";
import type {
  BreakdownBy,
  BreakdownResponse,
  ByteString,
  HistoricalRange,
  LiveSampleResponse,
  OverviewResponse,
  QualityResponse,
  SeriesPointResponse,
  SeriesResponse,
} from "../api/contracts";
import { asByteString } from "../lib/format";

const day = 86_400;

interface FixtureTemplates {
  breakdowns: Record<BreakdownBy, BreakdownResponse>;
}

interface Profile {
  seed: number;
  upload: bigint;
  download: bigint;
  previousPermille: number;
  qualityCount: number;
  coverage: number;
  retention: number;
  resolution: number;
  dataFrom: number;
  to: number;
  boundaryApproximate: boolean;
}

export interface MockHistory {
  overview: OverviewResponse;
  series: SeriesResponse;
  quality: QualityResponse;
  breakdowns: Record<BreakdownBy, BreakdownResponse>;
}

const templates = fixtureJSON as unknown as FixtureTemplates;

export function mockHistory(range: HistoricalRange, now: number): MockHistory {
  const profile = profileFor(range, now);
  const series = makeSeries(profile);
  const currentObserved = series.points.reduce(
    (sum, point) => sum + point.counter_observed_seconds,
    0,
  );
  const currentElapsed = series.points.reduce(
    (sum, point) => sum + point.elapsed_seconds,
    0,
  );
  const previousUpload =
    (profile.upload * BigInt(profile.previousPermille)) / 1000n;
  const previousDownload =
    (profile.download * BigInt(profile.previousPermille)) / 1000n;
  const overview: OverviewResponse = {
    current: {
      upload_bytes: bytes(profile.upload),
      download_bytes: bytes(profile.download),
      total_bytes: bytes(profile.upload + profile.download),
      elapsed_seconds: currentElapsed,
      observed_seconds: currentObserved,
    },
    previous: {
      upload_bytes: bytes(previousUpload),
      download_bytes: bytes(previousDownload),
      total_bytes: bytes(previousUpload + previousDownload),
      elapsed_seconds: profile.previousPermille === 0 ? 0 : currentElapsed,
      observed_seconds:
        profile.previousPermille === 0
          ? 0
          : Math.max(0, currentObserved - profile.qualityCount * 3),
    },
    boundary_approximate: profile.boundaryApproximate,
  };
  return {
    overview,
    series,
    quality: makeQuality(profile),
    breakdowns: Object.fromEntries(
      (Object.keys(templates.breakdowns) as BreakdownBy[]).map((by) => [
        by,
        makeBreakdown(by, profile),
      ]),
    ) as Record<BreakdownBy, BreakdownResponse>,
  };
}

export function generateLiveSamples(now: number): LiveSampleResponse[] {
  return Array.from({ length: 3600 }, (_, index) => {
    const cycle = index % 600;
    const triangle = cycle <= 300 ? cycle : 600 - cycle;
    const eveningBurst = index % 900 >= 690 ? 1_450_000 : 0;
    const sample: LiveSampleResponse = {
      timestamp: now - 3599 + index,
      upload_bytes_per_second:
        720_000 + triangle * 1_800 + ((index * 7_919) % 310_000),
      download_bytes_per_second:
        3_850_000 +
        triangle * 8_400 +
        ((index * 15_407) % 860_000) +
        eveningBurst,
      active_connections: 11 + ((index * 7 + Math.floor(index / 60)) % 18),
      status: index >= 1710 && index < 1722 ? "degraded" : "ok",
    };
    if (index === 3599) {
      sample.upload_bytes_per_second = 1_450_000;
      sample.download_bytes_per_second = 7_670_000;
      sample.active_connections = 17;
    }
    return sample;
  });
}

function profileFor(range: HistoricalRange, now: number): Profile {
  if (
    !Number.isSafeInteger(range.from) ||
    !Number.isSafeInteger(range.to) ||
    range.from <= 0 ||
    range.to <= range.from
  ) {
    throw new Error("invalid Demo historical range");
  }
  const duration = range.to - range.from;
  const todayStart = now - ((now + 8 * 3600) % day);
  if (range.from === day) {
    return fixedProfile(
      7,
      2_684_000_000_000n,
      9_742_000_000_000n,
      0,
      6,
      0.907,
      0.881,
      day,
      now - 560 * day,
      range.to,
      false,
    );
  }
  if (range.from === todayStart && range.to === now) {
    return fixedProfile(
      1,
      4_287_213_568n,
      12_884_901_888n,
      870,
      1,
      0.947,
      0.919,
      1800,
      range.from,
      range.to,
      false,
    );
  }
  if (range.to === todayStart && duration === day) {
    return fixedProfile(
      2,
      5_876_543_210n,
      17_456_789_012n,
      930,
      0,
      0.951,
      0.927,
      3600,
      range.from,
      range.to,
      false,
    );
  }
  if (range.to === now && duration === 7 * day) {
    return fixedProfile(
      3,
      42_700_000_000n,
      151_900_000_000n,
      910,
      2,
      0.938,
      0.908,
      3600,
      range.from,
      range.to,
      false,
    );
  }
  if (range.to === now && duration === 30 * day) {
    return fixedProfile(
      4,
      196_400_000_000n,
      713_800_000_000n,
      960,
      3,
      0.931,
      0.901,
      3600,
      range.from,
      range.to,
      false,
    );
  }
  if (range.to === now && duration === 90 * day) {
    return fixedProfile(
      5,
      612_700_000_000n,
      2_188_400_000_000n,
      890,
      4,
      0.924,
      0.893,
      day,
      range.from,
      range.to,
      false,
    );
  }
  if (range.to === now && duration > 90 * day && duration < 366 * day) {
    return fixedProfile(
      6,
      1_493_000_000_000n,
      5_482_000_000_000n,
      920,
      5,
      0.916,
      0.886,
      day,
      range.from,
      range.to,
      false,
    );
  }
  const days = Math.max(1, Math.ceil(duration / day));
  const offset = BigInt(Math.floor(Math.abs(range.from / day)) % 29);
  const resolution =
    duration <= 2 * day ? 1800 : duration <= 30 * day ? 3600 : day;
  return fixedProfile(
    8 + Number(offset),
    8_250_000_000n * BigInt(days) + offset * 19_000_000n,
    26_750_000_000n * BigInt(days) + offset * 43_000_000n,
    975,
    Math.min(4, Math.max(1, Math.ceil(days / 7))),
    0.929,
    0.897,
    resolution,
    range.from,
    range.to,
    true,
  );
}

function fixedProfile(
  seed: number,
  upload: bigint,
  download: bigint,
  previousPermille: number,
  qualityCount: number,
  coverage: number,
  retention: number,
  resolution: number,
  dataFrom: number,
  to: number,
  boundaryApproximate: boolean,
): Profile {
  return {
    seed,
    upload,
    download,
    previousPermille,
    qualityCount,
    coverage,
    retention,
    resolution,
    dataFrom,
    to,
    boundaryApproximate,
  };
}

function makeSeries(profile: Profile): SeriesResponse {
  const buckets: Array<{ start: number; end: number }> = [];
  for (let start = profile.dataFrom; start < profile.to; ) {
    const end = Math.min(profile.to, start + profile.resolution);
    buckets.push({ start, end });
    start = end;
  }
  const weights = buckets.map(({ start }, index) => {
    const hour = Math.floor(start / 3600) % 24;
    const daytime = hour >= 7 && hour <= 23 ? 38 : 4;
    return 72 + daytime + ((index * 37 + profile.seed * 11) % 47);
  });
  const uploads = allocate(profile.upload, weights);
  const downloads = allocate(
    profile.download,
    weights.map((value, index) => value + ((index * 13 + profile.seed) % 23)),
  );
  const flagged = new Set<number>();
  for (let index = 0; index < profile.qualityCount; index++) {
    flagged.add(
      Math.min(
        buckets.length - 1,
        Math.floor(((index + 1) * buckets.length) / (profile.qualityCount + 1)),
      ),
    );
  }
  const points: SeriesPointResponse[] = buckets.map((bucket, index) => {
    const elapsed = bucket.end - bucket.start;
    const hasQuality = flagged.has(index);
    const missing = hasQuality ? Math.min(elapsed - 1, 5 + (index % 17)) : 0;
    const upload = uploads[index]!;
    const download = downloads[index]!;
    const uploadRate = Number(upload / BigInt(elapsed));
    const downloadRate = Number(download / BigInt(elapsed));
    const samples = Math.max(1, Math.floor(elapsed / 10));
    const averageConnections = 9 + ((index * 5 + profile.seed) % 31);
    const recoveredUpload = hasQuality && index % 2 === 0 ? upload / 240n : 0n;
    const recoveredDownload =
      hasQuality && index % 2 === 0 ? download / 240n : 0n;
    return {
      bucket_start: bucket.start,
      bucket_end: bucket.end,
      elapsed_seconds: elapsed,
      source_resolution_sec: profile.resolution,
      upload_bytes: bytes(upload),
      download_bytes: bytes(download),
      recovered_upload_bytes: bytes(recoveredUpload),
      recovered_download_bytes: bytes(recoveredDownload),
      unattributed_upload_bytes: bytes(
        (upload * BigInt(1000 - Math.round(profile.coverage * 1000))) / 1000n,
      ),
      unattributed_download_bytes: bytes(
        (download * BigInt(1000 - Math.round(profile.coverage * 1000))) / 1000n,
      ),
      average_upload_bytes_per_second: uploadRate,
      average_download_bytes_per_second: downloadRate,
      peak_upload_bytes_per_second: uploadRate * (3 + (index % 3)),
      peak_download_bytes_per_second: downloadRate * (3 + ((index + 1) % 4)),
      counter_observed_seconds: elapsed - missing,
      active_connections_sum: averageConnections * samples,
      active_connections_samples: samples,
      active_connections_max: averageConnections + 5 + (index % 9),
      reset_count: hasQuality && index % 3 === 1 ? 1 : 0,
      quality_flags: hasQuality ? 1 << index % 4 : 0,
    };
  });
  return {
    boundary_approximate: profile.boundaryApproximate,
    points,
  };
}

function makeQuality(profile: Profile): QualityResponse {
  const codes = [
    "collector_quality",
    "counter_reset",
    "clash_unavailable",
    "storage_capacity_recovered",
  ];
  const duration = profile.to - profile.dataFrom;
  return {
    events: Array.from({ length: profile.qualityCount }, (_, index) => {
      const started =
        profile.dataFrom +
        Math.floor(((index + 1) * duration) / (profile.qualityCount + 1));
      return {
        code: codes[index % codes.length]!,
        started_at: started,
        ended_at: started + 8 + index * 3,
        flags: 1 << index % 4,
      };
    }),
  };
}

function makeBreakdown(by: BreakdownBy, profile: Profile): BreakdownResponse {
  const template = templates.breakdowns[by];
  const uploadParts = scaleTemplate(template, "upload_bytes", profile.upload);
  const downloadParts = scaleTemplate(
    template,
    "download_bytes",
    profile.download,
  );
  const itemCount = template.items.length;
  return {
    ...template,
    boundary_approximate: profile.boundaryApproximate,
    connection_coverage: profile.coverage,
    dimension_retention: profile.retention,
    global: {
      upload_bytes: bytes(profile.upload),
      download_bytes: bytes(profile.download),
    },
    other: {
      upload_bytes: uploadParts[itemCount]!,
      download_bytes: downloadParts[itemCount]!,
    },
    unattributed: {
      upload_bytes: uploadParts[itemCount + 1]!,
      download_bytes: downloadParts[itemCount + 1]!,
    },
    items: template.items.map((item, index) => ({
      ...item,
      upload_bytes: uploadParts[index]!,
      download_bytes: downloadParts[index]!,
    })),
  };
}

function scaleTemplate(
  template: BreakdownResponse,
  key: "upload_bytes" | "download_bytes",
  total: bigint,
): ByteString[] {
  const weights = [
    ...template.items.map((item) =>
      Number(BigInt(item[key]) / 1_000_000n + 1n),
    ),
    Number(BigInt(template.other[key]) / 1_000_000n + 1n),
    Number(BigInt(template.unattributed[key]) / 1_000_000n + 1n),
  ];
  return allocate(total, weights).map(bytes);
}

function allocate(total: bigint, weights: number[]): bigint[] {
  if (weights.length === 0) return [];
  const totalWeight = weights.reduce((sum, weight) => sum + BigInt(weight), 0n);
  let used = 0n;
  return weights.map((weight, index) => {
    const value =
      index === weights.length - 1
        ? total - used
        : (total * BigInt(weight)) / totalWeight;
    used += value;
    return value;
  });
}

function bytes(value: bigint): ByteString {
  return asByteString(value.toString());
}
