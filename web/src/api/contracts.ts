export type ByteString = string & { readonly __byteString: unique symbol };

export type TimeSelection =
  | { kind: "live" }
  | {
      kind: "preset";
      preset:
        | "today"
        | "yesterday"
        | "7d"
        | "30d"
        | "90d"
        | "year"
        | "lifetime";
    }
  | { kind: "custom"; from: string; to: string };

export type HistoricalRange = { from: number; to: number };
export type ServiceLevel = "ok" | "degraded" | "failed";

export interface Capabilities {
  connection_id: boolean;
  source: boolean;
  destination: boolean;
  port: boolean;
  network: boolean;
  domain: boolean;
}

export interface StatusResponse {
  status: ServiceLevel;
  reason: string;
  timezone: string;
  auth_enabled: boolean;
  capabilities: Capabilities;
}

export interface TotalsResponse {
  upload_bytes: ByteString;
  download_bytes: ByteString;
  total_bytes: ByteString;
  elapsed_seconds: number;
  observed_seconds: number;
}

export interface OverviewResponse {
  current: TotalsResponse;
  previous: TotalsResponse;
  boundary_approximate: boolean;
}

export interface SeriesPointResponse {
  bucket_start: number;
  bucket_end: number;
  elapsed_seconds: number;
  source_resolution_sec: number;
  upload_bytes: ByteString;
  download_bytes: ByteString;
  recovered_upload_bytes: ByteString;
  recovered_download_bytes: ByteString;
  unattributed_upload_bytes: ByteString;
  unattributed_download_bytes: ByteString;
  average_upload_bytes_per_second: number;
  average_download_bytes_per_second: number;
  speed_upload_sample_sum: ByteString;
  speed_download_sample_sum: ByteString;
  speed_sample_count: number;
  peak_upload_bytes_per_second: number;
  peak_download_bytes_per_second: number;
  counter_observed_seconds: number;
  active_connections_sum: number;
  active_connections_samples: number;
  active_connections_max: number;
  reset_count: number;
  quality_flags: number;
}

export interface SeriesResponse {
  boundary_approximate: boolean;
  points: SeriesPointResponse[];
}

export interface QualityEventResponse {
  code: string;
  started_at: number;
  ended_at: number | null;
  flags: number;
}

export interface QualityResponse {
  events: QualityEventResponse[];
}

export interface StorageResponse {
  database_bytes: ByteString;
  wal_bytes: ByteString;
  soft_limit_bytes: ByteString;
  protecting: boolean;
  last_rollup_cleanup: {
    started_at: number;
    ended_at: number | null;
    deleted_rows: number;
    successful: boolean;
  } | null;
}

export type BreakdownBy =
  | "target"
  | "endpoint"
  | "port"
  | "network"
  | "source"
  | "domain";

export interface BytePairResponse {
  upload_bytes: ByteString;
  download_bytes: ByteString;
}

export interface BreakdownItemResponse extends BytePairResponse {
  raw_value: string;
  display_name: string;
  network_code: number;
}

export interface BreakdownResponse {
  by: BreakdownBy;
  available: boolean;
  approximate: true;
  boundary_approximate: boolean;
  no_traffic: boolean;
  connection_coverage: number | null;
  dimension_retention: number | null;
  global: BytePairResponse;
  other: BytePairResponse;
  unattributed: BytePairResponse;
  items: BreakdownItemResponse[];
}

export interface LiveTargetResponse {
  raw_endpoint: string;
  display_name: string;
  network_code: number;
  host: string;
  upload_bytes_per_second: number;
  download_bytes_per_second: number;
}

export interface LiveTargetsResponse {
  observed_at: number;
  interval_millis: number;
  active_connections: number;
  global_upload_bytes_per_second: number;
  global_download_bytes_per_second: number;
  connection_coverage: number | null;
  targets: LiveTargetResponse[];
}

export interface RuntimeSessionResponse {
  started_at: number;
  ended_at: number | null;
  start_reason: string;
  end_reason: string | null;
  last_seen_at: number;
  sing_box_version: string;
  data_gap_before_seconds: number;
}

export interface LabelResponse {
  id: number;
  label_type: "host" | "endpoint";
  match_value: string;
  display_name: string;
  created_at: number;
  updated_at: number;
}

export interface LabelCandidateResponse {
  label_type: "host" | "endpoint";
  match_value: string;
  display_name: string;
  upload_bytes: ByteString;
  download_bytes: ByteString;
}

export interface LiveSampleResponse {
  timestamp: number;
  upload_bytes_per_second: number;
  download_bytes_per_second: number;
  active_connections: number;
  status: "ok" | "degraded";
}

export type LiveEvent =
  | { type: "snapshot"; sequence: number; samples: LiveSampleResponse[] }
  | { type: "sample"; sequence: number; sample: LiveSampleResponse }
  | {
      type: "status";
      sequence: number;
      status: ServiceLevel;
      reason: string;
      ready: boolean;
    }
  | { type: "heartbeat"; sequence: number; at: number };
