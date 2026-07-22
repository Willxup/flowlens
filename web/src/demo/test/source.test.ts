import { DemoDataSource, DemoReadOnlyError } from "../source";
import { generateLiveSamples } from "../mock-history";

describe("DemoDataSource", () => {
  it("generates continuous live traffic without sawtooth resets", () => {
    const samples = generateLiveSamples(1_800_000_000);
    const uploadSteps = samples
      .slice(1)
      .map((sample, index) =>
        Math.abs(
          sample.upload_bytes_per_second -
            samples[index]!.upload_bytes_per_second,
        ),
      );
    const downloadSteps = samples
      .slice(1)
      .map((sample, index) =>
        Math.abs(
          sample.download_bytes_per_second -
            samples[index]!.download_bytes_per_second,
        ),
      );

    expect(Math.max(...uploadSteps)).toBeLessThan(4_000);
    expect(Math.max(...downloadSteps)).toBeLessThan(10_000);
    expect(samples.at(-1)).toMatchObject({
      upload_bytes_per_second: 1_450_000,
      download_bytes_per_second: 7_670_000,
    });
  });

  it("is deterministic and never touches production networking", async () => {
    const fetchTrap = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValue(new Error("network forbidden"));
    const source = new DemoDataSource();

    const first = await source.series({ from: 1, to: 2 });
    const second = await source.series({ from: 1, to: 2 });
    expect(first).toEqual(second);
    expect(first).not.toBe(second);
    expect(fetchTrap).not.toHaveBeenCalled();
  });

  it("uses only fictional public-safe fixture values", async () => {
    const source = new DemoDataSource();
    const targets = await source.liveTargets();
    const fixtureText = JSON.stringify({
      targets,
      labels: await source.labels(),
      candidates: await source.labelCandidates(),
    });
    expect(fixtureText).toContain("example.test");
    expect(fixtureText).not.toMatch(
      /10\.\d+\.\d+\.\d+|192\.168\.|172\.(1[6-9]|2\d|3[01])\./,
    );
    expect(fixtureText).not.toContain("/Users/");
  });

  it("provides rich live and historical data with conserved byte totals", async () => {
    const source = new DemoDataSource();
    const liveTargets = await source.liveTargets();
    expect(liveTargets.targets).toHaveLength(6);
    const liveEvents: Parameters<
      Parameters<DemoDataSource["subscribeLive"]>[0]
    >[0][] = [];
    source.subscribeLive(
      (event) => liveEvents.push(event),
      () => undefined,
    );
    const snapshot = liveEvents.find((event) => event.type === "snapshot");
    expect(snapshot?.type).toBe("snapshot");
    if (snapshot?.type !== "snapshot") throw new Error("snapshot missing");
    expect(snapshot.samples).toHaveLength(3600);
    expect(
      snapshot.samples.every(
        (sample, index) =>
          index === 0 ||
          sample.timestamp - snapshot.samples[index - 1]!.timestamp === 1,
      ),
    ).toBe(true);
    const current = snapshot.samples.at(-1)!;
    const attributedRate = liveTargets.targets.reduce(
      (sum, target) =>
        sum + target.upload_bytes_per_second + target.download_bytes_per_second,
      0,
    );
    expect(
      attributedRate /
        (current.upload_bytes_per_second + current.download_bytes_per_second),
    ).toBeCloseTo(liveTargets.connection_coverage!, 3);

    for (const by of [
      "target",
      "endpoint",
      "port",
      "network",
      "source",
      "domain",
    ] as const) {
      const breakdown = await source.breakdown({ from: 1, to: 2 }, by);
      if (by === "target" || by === "endpoint" || by === "domain")
        expect(breakdown.items.length).toBeGreaterThanOrEqual(5);

      const upload = breakdown.items.reduce(
        (sum, item) => sum + BigInt(item.upload_bytes),
        BigInt(breakdown.other.upload_bytes) +
          BigInt(breakdown.unattributed.upload_bytes),
      );
      const download = breakdown.items.reduce(
        (sum, item) => sum + BigInt(item.download_bytes),
        BigInt(breakdown.other.download_bytes) +
          BigInt(breakdown.unattributed.download_bytes),
      );
      expect(upload).toBe(BigInt(breakdown.global.upload_bytes));
      expect(download).toBe(BigInt(breakdown.global.download_bytes));
    }
  });

  it("changes totals, series and quality by selected historical range", async () => {
    const source = new DemoDataSource();
    const now = Math.floor(source.now().getTime() / 1000);
    const ranges = [
      { name: "today", range: { from: now - 46_800, to: now } },
      {
        name: "yesterday",
        range: { from: now - 133_200, to: now - 46_800 },
      },
      { name: "7d", range: { from: now - 7 * 86_400, to: now } },
      { name: "30d", range: { from: now - 30 * 86_400, to: now } },
      { name: "90d", range: { from: now - 90 * 86_400, to: now } },
      { name: "year", range: { from: 1_767_196_800, to: now } },
      { name: "all", range: { from: 86_400, to: now } },
      {
        name: "custom",
        range: { from: now - 15 * 86_400, to: now - 86_400 },
      },
    ];
    const results = await Promise.all(
      ranges.map(async ({ name, range }) => ({
        name,
        overview: await source.overview(range),
        series: await source.series(range),
        quality: await source.quality(range),
      })),
    );

    expect(
      new Set(results.map((item) => item.overview.current.total_bytes)),
    ).toHaveLength(ranges.length);
    expect(
      new Set(results.map((item) => item.series.points.length)).size,
    ).toBeGreaterThanOrEqual(4);
    expect(
      new Set(results.map((item) => item.quality.events.length)).size,
    ).toBeGreaterThanOrEqual(3);
    for (const result of results) {
      expect(
        BigInt(result.overview.current.upload_bytes) +
          BigInt(result.overview.current.download_bytes),
      ).toBe(BigInt(result.overview.current.total_bytes));
      expect(
        result.series.points.reduce(
          (sum, point) => sum + BigInt(point.upload_bytes),
          0n,
        ),
      ).toBe(BigInt(result.overview.current.upload_bytes));
      expect(
        result.series.points.reduce(
          (sum, point) => sum + BigInt(point.download_bytes),
          0n,
        ),
      ).toBe(BigInt(result.overview.current.download_bytes));
    }
  });

  it("keeps every range-aware dimensional projection conserved", async () => {
    const source = new DemoDataSource();
    const now = Math.floor(source.now().getTime() / 1000);
    const ranges = [
      { from: now - 46_800, to: now },
      { from: now - 7 * 86_400, to: now },
      { from: now - 30 * 86_400, to: now },
      { from: 86_400, to: now },
      { from: now - 15 * 86_400, to: now - 86_400 },
    ];
    for (const range of ranges) {
      const overview = await source.overview(range);
      for (const by of [
        "target",
        "endpoint",
        "port",
        "network",
        "source",
        "domain",
      ] as const) {
        const breakdown = await source.breakdown(range, by);
        expect(breakdown.global.upload_bytes).toBe(
          overview.current.upload_bytes,
        );
        expect(breakdown.global.download_bytes).toBe(
          overview.current.download_bytes,
        );
        expect(
          breakdown.items.reduce(
            (sum, item) => sum + BigInt(item.upload_bytes),
            BigInt(breakdown.other.upload_bytes) +
              BigInt(breakdown.unattributed.upload_bytes),
          ),
        ).toBe(BigInt(breakdown.global.upload_bytes));
        expect(
          breakdown.items.reduce(
            (sum, item) => sum + BigInt(item.download_bytes),
            BigInt(breakdown.other.download_bytes) +
              BigInt(breakdown.unattributed.download_bytes),
          ),
        ).toBe(BigInt(breakdown.global.download_bytes));
      }
    }
  });

  it("keeps Demo alias writes read-only", async () => {
    const source = new DemoDataSource();
    await expect(
      source.createLabel({
        label_type: "host",
        match_value: "api.example.test",
        display_name: "API",
      }),
    ).rejects.toBeInstanceOf(DemoReadOnlyError);
  });
});
