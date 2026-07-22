import type { LiveEvent } from "../contracts";
import { ProductionDataSource } from "../production";

describe("ProductionDataSource live stream", () => {
  it("adds the named SSE event type and closes idempotently", () => {
    const stream = new FakeEventSource();
    const source = new ProductionDataSource(
      fetch,
      () => stream as unknown as EventSource,
    );
    const events: LiveEvent[] = [];
    const connection = vi.fn();

    const unsubscribe = source.subscribeLive(
      (event) => events.push(event),
      connection,
    );
    stream.open();
    stream.emit("snapshot", {
      sequence: 1,
      samples: [
        {
          timestamp: 1_700_000_000,
          upload_bytes_per_second: 12,
          download_bytes_per_second: 34,
          active_connections: 2,
          status: "ok",
        },
      ],
    });
    stream.emit("status", {
      sequence: 2,
      status: "ok",
      reason: "ready",
      ready: true,
    });

    expect(events).toEqual([
      expect.objectContaining({ type: "snapshot", sequence: 1 }),
      expect.objectContaining({ type: "status", sequence: 2 }),
    ]);
    expect(connection).toHaveBeenCalledWith(true);
    unsubscribe();
    unsubscribe();
    expect(stream.close).toHaveBeenCalledOnce();
  });

  it("rejects malformed or non-monotonic named events", () => {
    const stream = new FakeEventSource();
    const source = new ProductionDataSource(
      fetch,
      () => stream as unknown as EventSource,
    );
    const listener = vi.fn();
    const connection = vi.fn();
    source.subscribeLive(listener, connection);

    stream.emit("heartbeat", { sequence: 2, at: 1_700_000_000 });
    stream.emit("sample", { sequence: 2, sample: null });

    expect(listener).toHaveBeenCalledOnce();
    expect(listener).toHaveBeenCalledWith(
      expect.objectContaining({ type: "heartbeat", sequence: 2 }),
    );
    expect(connection).toHaveBeenLastCalledWith(false);
  });
});

class FakeEventSource {
  readonly listeners = new Map<string, EventListener>();
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  close = vi.fn();

  addEventListener(name: string, listener: EventListener) {
    this.listeners.set(name, listener);
  }

  open() {
    this.onopen?.(new Event("open"));
  }

  emit(name: string, body: unknown) {
    this.listeners.get(name)?.(
      new MessageEvent(name, { data: JSON.stringify(body) }),
    );
  }
}
