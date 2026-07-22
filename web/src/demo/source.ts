import fixtureJSON from "./fixture.json";
import type {
  BreakdownBy,
  BreakdownResponse,
  HistoricalRange,
  LabelCandidateResponse,
  LabelResponse,
  LiveEvent,
  LiveSampleResponse,
  LiveTargetsResponse,
  OverviewResponse,
  QualityResponse,
  RuntimeSessionResponse,
  SeriesResponse,
  StatusResponse,
  StorageResponse,
} from "../api/contracts";
import type { FlowLensDataSource, LabelInput } from "../api/source";
import { generateLiveSamples, mockHistory } from "./mock-history";

interface DemoFixture {
  now: number;
  status: StatusResponse;
  overview: OverviewResponse;
  series: SeriesResponse;
  quality: QualityResponse;
  storage: StorageResponse;
  breakdowns: Record<BreakdownBy, BreakdownResponse>;
  liveTargets: LiveTargetsResponse;
  liveSamples: LiveSampleResponse[];
  labels: LabelResponse[];
  candidates: LabelCandidateResponse[];
}

const fixture = fixtureJSON as unknown as DemoFixture;

export class DemoReadOnlyError extends Error {
  constructor() {
    super("FlowLens Demo is read-only");
    this.name = "DemoReadOnlyError";
  }
}

export class DemoDataSource implements FlowLensDataSource {
  readonly demo = true;

  now(): Date {
    return new Date(fixture.now * 1000);
  }

  async login(): Promise<void> {}
  async logout(): Promise<void> {}
  async status(): Promise<StatusResponse> {
    return clone(fixture.status);
  }
  async overview(_range: HistoricalRange): Promise<OverviewResponse> {
    return clone(mockHistory(_range, fixture.now).overview);
  }
  async series(_range: HistoricalRange): Promise<SeriesResponse> {
    return clone(mockHistory(_range, fixture.now).series);
  }
  async quality(_range: HistoricalRange): Promise<QualityResponse> {
    return clone(mockHistory(_range, fixture.now).quality);
  }
  async storage(): Promise<StorageResponse> {
    return clone(fixture.storage);
  }
  async breakdown(
    _range: HistoricalRange,
    by: BreakdownBy,
  ): Promise<BreakdownResponse> {
    return clone(mockHistory(_range, fixture.now).breakdowns[by]);
  }
  async liveTargets(): Promise<LiveTargetsResponse> {
    return clone(fixture.liveTargets);
  }
  async runtimeSessions(): Promise<RuntimeSessionResponse[]> {
    return clone(demoSessions);
  }
  async labels(): Promise<LabelResponse[]> {
    return clone(fixture.labels);
  }
  async labelCandidates(): Promise<LabelCandidateResponse[]> {
    return clone(fixture.candidates);
  }
  async createLabel(_input: LabelInput): Promise<LabelResponse> {
    throw new DemoReadOnlyError();
  }
  async updateLabel(_id: number, _displayName: string): Promise<LabelResponse> {
    throw new DemoReadOnlyError();
  }
  async deleteLabel(_id: number): Promise<void> {
    throw new DemoReadOnlyError();
  }

  subscribeLive(
    listener: (event: LiveEvent) => void,
    connection: (connected: boolean) => void,
  ): () => void {
    connection(true);
    listener({
      type: "snapshot",
      sequence: 1,
      samples: clone(demoLiveSamples),
    });
    listener({
      type: "status",
      sequence: 2,
      status: fixture.status.status,
      reason: fixture.status.reason,
      ready: true,
    });
    return () => connection(false);
  }
}

function clone<T>(value: T): T {
  return structuredClone(value);
}

const demoSessions: RuntimeSessionResponse[] = [
  {
    started_at: fixture.now - 73_200,
    ended_at: null,
    start_reason: "startup",
    end_reason: null,
    last_seen_at: fixture.now,
    sing_box_version: "sing-box 1.12.0",
    data_gap_before_seconds: 0,
  },
  {
    started_at: fixture.now - 248_400,
    ended_at: fixture.now - 73_260,
    start_reason: "startup",
    end_reason: "service_restart",
    last_seen_at: fixture.now - 73_260,
    sing_box_version: "sing-box 1.12.0",
    data_gap_before_seconds: 60,
  },
  {
    started_at: fixture.now - 507_600,
    ended_at: fixture.now - 248_520,
    start_reason: "counter_reset",
    end_reason: "counter_reset",
    last_seen_at: fixture.now - 248_520,
    sing_box_version: "sing-box 1.11.15",
    data_gap_before_seconds: 120,
  },
];

const demoLiveSamples = generateLiveSamples(fixture.now);
