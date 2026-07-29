import type {
  BreakdownBy,
  BreakdownResponse,
  HistoricalSelection,
  LabelCandidateResponse,
  LabelResponse,
  LiveEvent,
  LiveTargetsResponse,
  OverviewResponse,
  QualityResponse,
  RuntimeSessionResponse,
  SeriesResponse,
  StatusResponse,
  StorageResponse,
} from "./contracts";

export interface LabelInput {
  label_type: "host" | "endpoint";
  match_value: string;
  display_name: string;
}

export interface FlowLensDataSource {
  readonly demo: boolean;
  now(): Date;
  login(accessKey: string, signal?: AbortSignal): Promise<void>;
  logout(signal?: AbortSignal): Promise<void>;
  status(signal?: AbortSignal): Promise<StatusResponse>;
  overview(
    range: HistoricalSelection,
    signal?: AbortSignal,
  ): Promise<OverviewResponse>;
  series(
    range: HistoricalSelection,
    signal?: AbortSignal,
  ): Promise<SeriesResponse>;
  quality(
    range: HistoricalSelection,
    signal?: AbortSignal,
  ): Promise<QualityResponse>;
  storage(signal?: AbortSignal): Promise<StorageResponse>;
  breakdown(
    range: HistoricalSelection,
    by: BreakdownBy,
    signal?: AbortSignal,
  ): Promise<BreakdownResponse>;
  liveTargets(signal?: AbortSignal): Promise<LiveTargetsResponse>;
  runtimeSessions(signal?: AbortSignal): Promise<RuntimeSessionResponse[]>;
  labels(signal?: AbortSignal): Promise<LabelResponse[]>;
  labelCandidates(signal?: AbortSignal): Promise<LabelCandidateResponse[]>;
  createLabel(input: LabelInput, signal?: AbortSignal): Promise<LabelResponse>;
  updateLabel(
    id: number,
    displayName: string,
    signal?: AbortSignal,
  ): Promise<LabelResponse>;
  deleteLabel(id: number, signal?: AbortSignal): Promise<void>;
  subscribeLive(
    listener: (event: LiveEvent) => void,
    connection: (connected: boolean) => void,
  ): () => void;
}
