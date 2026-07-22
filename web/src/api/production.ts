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
} from "./contracts";
import type { FlowLensDataSource, LabelInput } from "./source";

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export class UnauthorizedError extends ApiError {
  constructor() {
    super("FlowLens session is unavailable", 401);
    this.name = "UnauthorizedError";
  }
}

type Fetcher = typeof fetch;
type EventSourceFactory = (url: string) => EventSource;

export class ProductionDataSource implements FlowLensDataSource {
  readonly demo = false;

  constructor(
    private readonly fetcher: Fetcher = (input, init) => fetch(input, init),
    private readonly eventSourceFactory: EventSourceFactory = (url) =>
      new EventSource(url),
  ) {}

  now(): Date {
    return new Date();
  }

  async login(accessKey: string, signal?: AbortSignal): Promise<void> {
    await this.request("/api/v1/session", undefined, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ access_key: accessKey }),
      signal,
    });
  }

  async logout(signal?: AbortSignal): Promise<void> {
    await this.request("/api/v1/session", undefined, {
      method: "DELETE",
      signal,
    });
  }

  status(signal?: AbortSignal): Promise<StatusResponse> {
    return this.request("/api/v1/status", "status", { signal });
  }

  overview(
    range: HistoricalRange,
    signal?: AbortSignal,
  ): Promise<OverviewResponse> {
    return this.request(`/api/v1/overview?${rangeQuery(range)}`, "overview", {
      signal,
    });
  }

  series(
    range: HistoricalRange,
    signal?: AbortSignal,
  ): Promise<SeriesResponse> {
    return this.request(
      `/api/v1/series?${rangeQuery(range)}&resolution=auto`,
      "series",
      { signal },
    );
  }

  quality(
    range: HistoricalRange,
    signal?: AbortSignal,
  ): Promise<QualityResponse> {
    return this.request(`/api/v1/quality?${rangeQuery(range)}`, "quality", {
      signal,
    });
  }

  storage(signal?: AbortSignal): Promise<StorageResponse> {
    return this.request("/api/v1/storage", "storage", { signal });
  }

  breakdown(
    range: HistoricalRange,
    by: BreakdownBy,
    signal?: AbortSignal,
  ): Promise<BreakdownResponse> {
    return this.request(
      `/api/v1/breakdown?${rangeQuery(range)}&by=${encodeURIComponent(by)}`,
      "breakdown",
      { signal },
    );
  }

  liveTargets(signal?: AbortSignal): Promise<LiveTargetsResponse> {
    return this.request("/api/v1/connections/live", "live-targets", { signal });
  }

  async runtimeSessions(
    signal?: AbortSignal,
  ): Promise<RuntimeSessionResponse[]> {
    const result = await this.request<{ sessions: RuntimeSessionResponse[] }>(
      "/api/v1/runtime-sessions",
      "runtime-sessions",
      { signal },
    );
    return result.sessions;
  }

  async labels(signal?: AbortSignal): Promise<LabelResponse[]> {
    const result = await this.request<{ labels: LabelResponse[] }>(
      "/api/v1/labels",
      "labels",
      { signal },
    );
    return result.labels;
  }

  async labelCandidates(
    signal?: AbortSignal,
  ): Promise<LabelCandidateResponse[]> {
    const result = await this.request<{ candidates: LabelCandidateResponse[] }>(
      "/api/v1/label-candidates",
      "candidates",
      { signal },
    );
    return result.candidates;
  }

  createLabel(input: LabelInput, signal?: AbortSignal): Promise<LabelResponse> {
    return this.request(
      "/api/v1/labels",
      "label",
      jsonWrite("POST", input, signal),
    );
  }

  updateLabel(
    id: number,
    displayName: string,
    signal?: AbortSignal,
  ): Promise<LabelResponse> {
    return this.request(
      `/api/v1/labels/${id}`,
      "label",
      jsonWrite("PUT", { display_name: displayName }, signal),
    );
  }

  async deleteLabel(id: number, signal?: AbortSignal): Promise<void> {
    await this.request(`/api/v1/labels/${id}`, undefined, {
      method: "DELETE",
      signal,
    });
  }

  subscribeLive(
    listener: (event: LiveEvent) => void,
    connection: (connected: boolean) => void,
  ): () => void {
    const stream = this.eventSourceFactory("/api/v1/live");
    let lastSequence = 0;
    const names: LiveEvent["type"][] = [
      "snapshot",
      "sample",
      "status",
      "heartbeat",
    ];
    for (const name of names) {
      const receive = (event: MessageEvent<string>) => {
        try {
          const parsed = parseLiveEvent(name, event.data, lastSequence);
          if (parsed === null) throw new Error("invalid live event");
          lastSequence = parsed.sequence;
          listener(parsed);
        } catch {
          connection(false);
        }
      };
      stream.addEventListener(name, receive as EventListener);
    }
    stream.onopen = () => {
      lastSequence = 0;
      connection(true);
    };
    stream.onerror = () => connection(false);
    let closed = false;
    return () => {
      if (closed) return;
      closed = true;
      stream.close();
    };
  }

  private async request<T>(
    path: string,
    shape: string | undefined,
    init: RequestInit = {},
  ): Promise<T> {
    const response = await this.fetcher(path, {
      credentials: "same-origin",
      ...init,
    });
    if (response.status === 401) throw new UnauthorizedError();
    if (!response.ok)
      throw new ApiError("FlowLens request failed", response.status);
    if (response.status === 204) return undefined as T;
    let value: unknown;
    try {
      value = await response.json();
    } catch {
      throw new Error("FlowLens response is invalid");
    }
    if (!validResponse(value, shape))
      throw new Error("FlowLens response is invalid");
    return value as T;
  }
}

function parseLiveEvent(
  type: LiveEvent["type"],
  data: string,
  lastSequence: number,
): LiveEvent | null {
  const value: unknown = JSON.parse(data);
  if (
    !isRecord(value) ||
    !Number.isSafeInteger(value.sequence) ||
    (value.sequence as number) <= lastSequence
  )
    return null;
  const sequence = value.sequence as number;
  switch (type) {
    case "snapshot":
      return Array.isArray(value.samples) &&
        value.samples.every(validLiveSample)
        ? { type, sequence, samples: value.samples }
        : null;
    case "sample":
      return validLiveSample(value.sample)
        ? { type, sequence, sample: value.sample }
        : null;
    case "status":
      return validServiceLevel(value.status) &&
        typeof value.reason === "string" &&
        typeof value.ready === "boolean"
        ? {
            type,
            sequence,
            status: value.status,
            reason: value.reason,
            ready: value.ready,
          }
        : null;
    case "heartbeat":
      return Number.isSafeInteger(value.at) && (value.at as number) > 0
        ? { type, sequence, at: value.at as number }
        : null;
  }
}

function validLiveSample(value: unknown): value is LiveSampleResponse {
  return (
    isRecord(value) &&
    Number.isSafeInteger(value.timestamp) &&
    (value.timestamp as number) > 0 &&
    nonnegativeInteger(value.upload_bytes_per_second) &&
    nonnegativeInteger(value.download_bytes_per_second) &&
    nonnegativeInteger(value.active_connections) &&
    (value.status === "ok" || value.status === "degraded")
  );
}

function nonnegativeInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) >= 0;
}

function validServiceLevel(value: unknown): value is StatusResponse["status"] {
  return value === "ok" || value === "degraded" || value === "failed";
}

function rangeQuery(range: HistoricalRange): string {
  return `from=${range.from}&to=${range.to}`;
}

function jsonWrite(
  method: string,
  value: unknown,
  signal?: AbortSignal,
): RequestInit {
  return {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(value),
    signal,
  };
}

function validResponse(value: unknown, shape: string | undefined): boolean {
  if (!isRecord(value) || !validJSON(value, "")) return false;
  switch (shape) {
    case "status":
      return (
        typeof value.status === "string" &&
        typeof value.reason === "string" &&
        typeof value.timezone === "string" &&
        isRecord(value.capabilities)
      );
    case "overview":
      return (
        isRecord(value.current) &&
        isRecord(value.previous) &&
        typeof value.boundary_approximate === "boolean"
      );
    case "series":
      return (
        Array.isArray(value.points) &&
        typeof value.boundary_approximate === "boolean"
      );
    case "quality":
      return Array.isArray(value.events);
    case "storage":
      return typeof value.protecting === "boolean";
    case "breakdown":
      return (
        Array.isArray(value.items) &&
        isRecord(value.global) &&
        isRecord(value.other) &&
        isRecord(value.unattributed)
      );
    case "live-targets":
      return (
        Array.isArray(value.targets) &&
        typeof value.observed_at === "number" &&
        nonnegativeInteger(value.global_upload_bytes_per_second) &&
        nonnegativeInteger(value.global_download_bytes_per_second)
      );
    case "runtime-sessions":
      return Array.isArray(value.sessions);
    case "labels":
      return Array.isArray(value.labels);
    case "candidates":
      return Array.isArray(value.candidates);
    case "label":
      return (
        typeof value.id === "number" && typeof value.display_name === "string"
      );
    default:
      return true;
  }
}

function validJSON(value: unknown, key: string): boolean {
  if (key.endsWith("_bytes"))
    return typeof value === "string" && /^(0|[1-9][0-9]*)$/.test(value);
  if (value === null || typeof value === "string" || typeof value === "boolean")
    return true;
  if (typeof value === "number") {
    if (key === "connection_coverage" || key === "dimension_retention")
      return Number.isFinite(value) && value >= 0 && value <= 1;
    return Number.isSafeInteger(value);
  }
  if (Array.isArray(value)) return value.every((item) => validJSON(item, ""));
  if (!isRecord(value)) return false;
  return Object.entries(value).every(([childKey, child]) =>
    validJSON(child, childKey),
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
