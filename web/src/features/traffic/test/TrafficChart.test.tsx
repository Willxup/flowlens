import { render, waitFor } from "@testing-library/react";
import type { ByteString } from "../../../api/contracts";
import type { HistoricalChartPoint } from "../../history/model";
import { TrafficChart } from "../TrafficChart";

const { chart, init } = vi.hoisted(() => {
  const chartInstance = {
    setOption: vi.fn(),
    resize: vi.fn(),
    dispose: vi.fn(),
  };
  return { chart: chartInstance, init: vi.fn(() => chartInstance) };
});

vi.mock("echarts/core", () => ({
  init,
  use: vi.fn(),
}));
vi.mock("echarts/charts", () => ({ BarChart: {}, LineChart: {} }));
vi.mock("echarts/components", () => ({
  GridComponent: {},
  LegendComponent: {},
  TooltipComponent: {},
}));
vi.mock("echarts/renderers", () => ({ SVGRenderer: {} }));

describe("TrafficChart", () => {
  beforeAll(() => {
    Object.defineProperty(HTMLDivElement.prototype, "clientWidth", {
      configurable: true,
      get: () => 800,
    });
    vi.stubGlobal(
      "matchMedia",
      vi.fn(
        (media: string): MediaQueryList => ({
          matches: false,
          media,
          onchange: null,
          addListener: vi.fn(),
          removeListener: vi.fn(),
          addEventListener: vi.fn(),
          removeEventListener: vi.fn(),
          dispatchEvent: vi.fn(),
        }),
      ),
    );
  });

  beforeEach(() => {
    init.mockClear();
    chart.setOption.mockClear();
    chart.resize.mockClear();
    chart.dispose.mockClear();
  });

  afterAll(() => {
    Reflect.deleteProperty(HTMLDivElement.prototype, "clientWidth");
    vi.unstubAllGlobals();
  });

  it("renders live speed as smooth monotone curves without bridging gaps", async () => {
    render(
      <TrafficChart
        mode="live"
        live={[
          { timestamp: 1, upload: 1, download: 2 },
          { timestamp: 2, upload: null, download: null },
          { timestamp: 3, upload: 3, download: 5 },
        ]}
      />,
    );
    await waitFor(() => expect(chart.setOption).toHaveBeenCalledOnce());

    const option = chart.setOption.mock.calls[0]![0] as {
      animation: boolean;
      xAxis: { axisLabel: { show: boolean } };
      yAxis: { axisLabel: { formatter: (value: number) => string } };
      series: Array<Record<string, unknown>>;
    };
    expect(option.animation).toBe(false);
    expect(option.xAxis.axisLabel.show).toBe(true);
    expect(option.yAxis.axisLabel.formatter(2048)).toBe("2 KiB/s");
    expect(option.series).toEqual([
      expect.objectContaining({
        name: "下载",
        smooth: 0.45,
        smoothMonotone: "x",
        connectNulls: false,
      }),
      expect.objectContaining({
        name: "上传",
        smooth: 0.45,
        smoothMonotone: "x",
        connectNulls: false,
      }),
    ]);
  });

  it("updates live data without rebuilding the chart instance", async () => {
    const { rerender, unmount } = render(
      <TrafficChart
        mode="live"
        live={[{ timestamp: 1, upload: 1, download: 2 }]}
      />,
    );
    await waitFor(() => expect(chart.setOption).toHaveBeenCalledOnce());

    rerender(
      <TrafficChart
        mode="live"
        live={[
          { timestamp: 1, upload: 1, download: 2 },
          { timestamp: 2, upload: 2, download: 4 },
        ]}
      />,
    );
    await waitFor(() => expect(chart.setOption).toHaveBeenCalledTimes(2));

    expect(init).toHaveBeenCalledOnce();
    expect(chart.dispose).not.toHaveBeenCalled();
    const firstSeries = chart.setOption.mock.calls[0]![0] as {
      series: Array<{ id?: string }>;
    };
    const secondUpdateOptions = chart.setOption.mock.calls[1]![1];
    expect(firstSeries.series.map((series) => series.id)).toEqual([
      "live-download",
      "live-upload",
    ]);
    expect(secondUpdateOptions).toEqual({ lazyUpdate: true });
    unmount();
    expect(chart.dispose).toHaveBeenCalledOnce();
  });

  it("limits realtime time labels to a compact set", async () => {
    render(
      <TrafficChart
        mode="live"
        live={Array.from({ length: 40 }, (_, index) => ({
          timestamp: index + 1,
          upload: index,
          download: index * 2,
        }))}
      />,
    );
    await waitFor(() => expect(chart.setOption).toHaveBeenCalledOnce());

    const option = chart.setOption.mock.calls[0]![0] as {
      xAxis: { axisLabel: { interval: number } };
    };
    expect(option.xAxis.axisLabel.interval).toBe(3);
  });

  it("uses hour labels for day history and date labels for longer history", async () => {
    const history: HistoricalChartPoint[] = [
      {
        start: 1_721_284_200,
        end: 1_721_287_800,
        upload: "1" as ByteString,
        download: "2" as ByteString,
        cumulative: "3" as ByteString,
        uploadRate: 1,
        downloadRate: 2,
        resolution: 3_600,
      },
    ];
    const { rerender } = render(
      <TrafficChart mode="history" history={history} historyLabelMode="time" />,
    );
    await waitFor(() => expect(chart.setOption).toHaveBeenCalledOnce());

    const dayOption = chart.setOption.mock.calls.at(-1)![0] as {
      xAxis: { axisLabel: { formatter: (value: string) => string } };
    };
    expect(
      dayOption.xAxis.axisLabel.formatter(String(history[0]!.start)),
    ).toMatch(/^\d{2}:\d{2}$/);

    rerender(
      <TrafficChart mode="history" history={history} historyLabelMode="date" />,
    );
    await waitFor(() => expect(chart.setOption).toHaveBeenCalledTimes(2));
    const longerOption = chart.setOption.mock.calls.at(-1)![0] as {
      xAxis: { axisLabel: { formatter: (value: string) => string } };
    };
    expect(
      longerOption.xAxis.axisLabel.formatter(String(history[0]!.start)),
    ).not.toContain(":");
  });

  it("smooths historical speed but leaves cumulative traffic unsmoothed", async () => {
    const history: HistoricalChartPoint[] = [
      {
        start: 1,
        end: 2,
        upload: "1" as ByteString,
        download: "2" as ByteString,
        cumulative: "3" as ByteString,
        uploadRate: 1,
        downloadRate: 2,
        resolution: 1,
      },
    ];
    const { rerender } = render(
      <TrafficChart mode="history" historyView="speed" history={history} />,
    );
    await waitFor(() => expect(chart.setOption).toHaveBeenCalledOnce());

    const speed = chart.setOption.mock.calls.at(-1)![0] as {
      series: Array<Record<string, unknown>>;
    };
    expect(speed.series).toEqual([
      expect.objectContaining({ smooth: 0.45, smoothMonotone: "x" }),
      expect.objectContaining({ smooth: 0.45, smoothMonotone: "x" }),
    ]);

    rerender(
      <TrafficChart mode="history" historyView="traffic" history={history} />,
    );
    await waitFor(() => expect(chart.setOption).toHaveBeenCalledTimes(2));
    const traffic = chart.setOption.mock.calls.at(-1)![0] as {
      series: Array<Record<string, unknown>>;
    };
    expect(traffic.series[2]).not.toHaveProperty("smooth");
    expect(chart.setOption.mock.calls.at(-1)![1]).toEqual({
      lazyUpdate: true,
      replaceMerge: ["series"],
    });
  });
});
