import { asByteString } from "../../lib/format";
import {
  ApiError,
  ProductionDataSource,
  UnauthorizedError,
} from "../production";

describe("ProductionDataSource", () => {
  it("sends the access key only in the fixed login request", async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }));
    const source = new ProductionDataSource(fetcher as typeof fetch);

    await source.login("fixture-access-key");

    expect(fetcher).toHaveBeenCalledOnce();
    const calls = fetcher.mock.calls as unknown as Array<
      [RequestInfo | URL, RequestInit?]
    >;
    const call = calls[0];
    expect(call).toBeDefined();
    const [url, init] = call as [RequestInfo | URL, RequestInit?];
    expect(url).toBe("/api/v1/session");
    expect(init).toMatchObject({
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
    expect(JSON.parse(String(init?.body))).toEqual({
      access_key: "fixture-access-key",
    });
    expect(JSON.stringify(source)).not.toContain("fixture-access-key");
  });

  it("builds exact historical query parameters and preserves byte strings", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      expect(String(input)).toBe(
        "/api/v1/series?from=10&to=20&resolution=auto",
      );
      return Response.json({ boundary_approximate: false, points: [] });
    });
    const source = new ProductionDataSource(fetcher as typeof fetch);

    const value = await source.series({ from: 10, to: 20 });

    expect(value).toEqual({ boundary_approximate: false, points: [] });
    expect(asByteString("9007199254740993")).toBe("9007199254740993");
  });

  it("classifies unauthorized and generic service failures", async () => {
    const unauthorized = new ProductionDataSource(
      vi.fn(async () => new Response(null, { status: 401 })) as typeof fetch,
    );
    await expect(unauthorized.status()).rejects.toBeInstanceOf(
      UnauthorizedError,
    );

    const unavailable = new ProductionDataSource(
      vi.fn(
        async () => new Response("private detail", { status: 503 }),
      ) as typeof fetch,
    );
    await expect(unavailable.storage()).rejects.toEqual(
      new ApiError("FlowLens request failed", 503),
    );
  });

  it("rejects malformed JSON DTOs", async () => {
    const source = new ProductionDataSource(
      vi.fn(async () =>
        Response.json({
          database_bytes: 42,
          wal_bytes: "0",
          soft_limit_bytes: "1",
          protecting: false,
        }),
      ) as typeof fetch,
    );
    await expect(source.storage()).rejects.toThrow(
      "FlowLens response is invalid",
    );
  });

  it("accepts the two documented fractional breakdown ratios", async () => {
    const source = new ProductionDataSource(
      vi.fn(async () =>
        Response.json({
          by: "endpoint",
          available: true,
          approximate: true,
          boundary_approximate: false,
          no_traffic: false,
          connection_coverage: 0.924,
          dimension_retention: 0.884,
          global: { upload_bytes: "10", download_bytes: "20" },
          other: { upload_bytes: "0", download_bytes: "0" },
          unattributed: { upload_bytes: "1", download_bytes: "2" },
          items: [],
        }),
      ) as typeof fetch,
    );

    await expect(
      source.breakdown({ from: 10, to: 20 }, "endpoint"),
    ).resolves.toMatchObject({
      connection_coverage: 0.924,
      dimension_retention: 0.884,
    });
  });

  it("loads the existing public runtime-session endpoint", async () => {
    const payload = {
      sessions: [
        {
          started_at: 1_784_480_400,
          ended_at: null,
          start_reason: "startup",
          end_reason: null,
          last_seen_at: 1_784_523_600,
          sing_box_version: "sing-box 1.12.0",
          data_gap_before_seconds: 0,
        },
      ],
    };
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      expect(String(input)).toBe("/api/v1/runtime-sessions");
      return Response.json(payload);
    });
    const source = new ProductionDataSource(fetcher as typeof fetch);

    await expect(source.runtimeSessions()).resolves.toEqual(payload.sessions);
  });

  it("rejects live target payloads without same-window global rates", async () => {
    const source = new ProductionDataSource(
      vi.fn(async () =>
        Response.json({
          observed_at: 100,
          interval_millis: 1000,
          active_connections: 1,
          connection_coverage: 1,
          targets: [],
        }),
      ) as typeof fetch,
    );

    await expect(source.liveTargets()).rejects.toThrow(
      "FlowLens response is invalid",
    );
  });
});
