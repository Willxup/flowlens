import { useEffect, useRef } from "react";
import * as echarts from "echarts/core";
import { BarChart, LineChart } from "echarts/charts";
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
} from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import type { LiveChartPoint } from "../live/model";
import type { HistoricalChartPoint } from "../history/model";
import { formatRate } from "../../lib/format";

echarts.use([
  LineChart,
  BarChart,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  SVGRenderer,
]);

interface TrafficChartProps {
  mode: "live" | "history";
  historyView?: "traffic" | "speed";
  historyLabelMode?: "time" | "date";
  live?: LiveChartPoint[];
  history?: HistoricalChartPoint[];
}

export function TrafficChart({
  mode,
  historyView = "traffic",
  historyLabelMode = "date",
  live = [],
  history = [],
}: TrafficChartProps) {
  const reference = useRef<HTMLDivElement>(null);
  const chartReference = useRef<ReturnType<typeof echarts.init> | null>(null);
  const seriesShapeReference = useRef<string | null>(null);
  useEffect(() => {
    const element = reference.current;
    if (element === null || element.clientWidth === 0) return;
    const chart = echarts.init(element, undefined, { renderer: "svg" });
    chartReference.current = chart;
    const resize = () => chart.resize();
    window.addEventListener("resize", resize);
    return () => {
      window.removeEventListener("resize", resize);
      chartReference.current = null;
      chart.dispose();
    };
  }, []);

  useEffect(() => {
    const chart = chartReference.current;
    if (chart === null) return;
    const labels =
      mode === "live"
        ? live.map((point) => point.timestamp)
        : history.map((point) => point.start);
    const speedScale = mode === "live" || historyView === "speed";
    const seriesShape = mode === "live" ? "live" : `history-${historyView}`;
    const replaceSeries =
      seriesShapeReference.current !== null &&
      seriesShapeReference.current !== seriesShape;
    seriesShapeReference.current = seriesShape;
    chart.setOption(
      {
        animation:
          mode !== "live" &&
          !window.matchMedia("(prefers-reduced-motion: reduce)").matches,
        color: ["#6d5dfc", "#f29a3f", "#24a99a"],
        grid: { left: 52, right: 14, top: 34, bottom: 28 },
        tooltip: {
          trigger: "axis",
          backgroundColor: "rgba(20, 21, 24, 0.94)",
          borderColor: "rgba(127, 127, 127, 0.24)",
          textStyle: { color: "#f7f7f5", fontSize: 11 },
        },
        legend: {
          top: 0,
          right: 4,
          itemWidth: 14,
          itemHeight: 7,
          textStyle: { color: "#7b7d82", fontSize: 10 },
        },
        xAxis: {
          type: "category",
          data: labels,
          boundaryGap: mode === "history" && historyView === "traffic",
          axisTick: { show: false },
          axisLine: { lineStyle: { color: "rgba(127, 127, 127, 0.2)" } },
          axisLabel: {
            show: true,
            hideOverlap: true,
            interval: Math.max(
              0,
              Math.ceil(labels.length / (mode === "live" ? 10 : 8)) - 1,
            ),
            color: "#8a8c91",
            fontSize: 10,
            formatter: (value: string) =>
              formatTimestamp(value, mode, historyLabelMode),
          },
        },
        yAxis: {
          type: "value",
          axisLabel: {
            color: "#8a8c91",
            fontSize: 10,
            formatter: speedScale ? formatRate : formatTrafficScale,
          },
          axisLine: { show: false },
          axisTick: { show: false },
          splitLine: { lineStyle: { color: "rgba(127, 127, 127, 0.16)" } },
        },
        series:
          mode === "live"
            ? [
                {
                  id: "live-download",
                  name: "下载",
                  type: "line",
                  showSymbol: false,
                  smooth: 0.45,
                  smoothMonotone: "x",
                  connectNulls: false,
                  lineStyle: { width: 2 },
                  areaStyle: { opacity: 0.07 },
                  data: live.map((point) => point.download),
                },
                {
                  id: "live-upload",
                  name: "上传",
                  type: "line",
                  showSymbol: false,
                  smooth: 0.45,
                  smoothMonotone: "x",
                  connectNulls: false,
                  lineStyle: { width: 2 },
                  areaStyle: { opacity: 0.05 },
                  data: live.map((point) => point.upload),
                },
              ]
            : historyView === "speed"
              ? [
                  {
                    id: "history-speed-download",
                    name: "平均下载",
                    type: "line",
                    showSymbol: false,
                    smooth: 0.45,
                    smoothMonotone: "x",
                    connectNulls: false,
                    lineStyle: { width: 2 },
                    areaStyle: { opacity: 0.07 },
                    data: history.map((point) => point.downloadRate),
                  },
                  {
                    id: "history-speed-upload",
                    name: "平均上传",
                    type: "line",
                    showSymbol: false,
                    smooth: 0.45,
                    smoothMonotone: "x",
                    connectNulls: false,
                    lineStyle: { width: 2 },
                    areaStyle: { opacity: 0.05 },
                    data: history.map((point) => point.uploadRate),
                  },
                ]
              : [
                  {
                    id: "history-traffic-download",
                    name: "下载",
                    type: "bar",
                    stack: "bytes",
                    data: history.map((point) =>
                      Number(BigInt(point.download) / 1024n),
                    ),
                  },
                  {
                    id: "history-traffic-upload",
                    name: "上传",
                    type: "bar",
                    stack: "bytes",
                    data: history.map((point) =>
                      Number(BigInt(point.upload) / 1024n),
                    ),
                  },
                  {
                    id: "history-traffic-cumulative",
                    name: "累计",
                    type: "line",
                    yAxisIndex: 0,
                    showSymbol: false,
                    data: history.map((point) =>
                      Number(BigInt(point.cumulative) / 1024n),
                    ),
                  },
                ],
      },
      replaceSeries
        ? { lazyUpdate: true, replaceMerge: ["series"] }
        : { lazyUpdate: true },
    );
  }, [history, historyLabelMode, historyView, live, mode]);
  return (
    <div
      ref={reference}
      className="chart-shell"
      role="img"
      aria-label={
        mode === "live"
          ? "实时上传和下载速度曲线"
          : historyView === "speed"
            ? "历史平均上传和下载速度曲线"
            : "历史上传下载流量和累计曲线"
      }
    />
  );
}

function formatTimestamp(
  value: string,
  mode: "live" | "history",
  historyLabelMode: "time" | "date",
): string {
  const timestamp = Number(value);
  if (!Number.isFinite(timestamp)) return "";
  const date = new Date(timestamp * 1000);
  if (mode === "live" || historyLabelMode === "time") {
    return date.toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    });
  }
  return date.toLocaleDateString("zh-CN", { month: "numeric", day: "numeric" });
}

function formatTrafficScale(value: number): string {
  if (!Number.isFinite(value) || value < 0) return "—";
  const units = ["KiB", "MiB", "GiB", "TiB", "PiB"];
  let scaled = value;
  let unit = 0;
  while (scaled >= 1024 && unit < units.length - 1) {
    scaled /= 1024;
    unit++;
  }
  const digits = scaled >= 10 || Number.isInteger(scaled) ? 0 : 1;
  return `${scaled.toFixed(digits)} ${units[unit]}`;
}
