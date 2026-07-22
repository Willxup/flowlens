import { render, waitFor } from "@testing-library/react";
import type { ByteString } from "../../../api/contracts";
import type { HistoricalChartPoint } from "../../history/model";
import { TrafficChart } from "../TrafficChart";

const chart = vi.hoisted(() => ({
  setOption: vi.fn(),
  resize: vi.fn(),
  dispose: vi.fn(),
}));

vi.mock("echarts/core", () => ({
  init: vi.fn(() => chart),
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
    chart.setOption.mockClear();
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
      series: Array<Record<string, unknown>>;
    };
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
  });
});
