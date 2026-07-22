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
  live?: LiveChartPoint[];
  history?: HistoricalChartPoint[];
}

export function TrafficChart({
  mode,
  historyView = "traffic",
  live = [],
  history = [],
}: TrafficChartProps) {
  const reference = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = reference.current;
    if (element === null || element.clientWidth === 0) return;
    const chart = echarts.init(element, undefined, { renderer: "svg" });
    const labels =
      mode === "live"
        ? live.map((point) => point.timestamp)
        : history.map((point) => point.start);
    chart.setOption({
      animation: !window.matchMedia("(prefers-reduced-motion: reduce)").matches,
      color: ["#6d5dfc", "#f29a3f", "#24a99a"],
      grid: { left: 46, right: 18, top: 28, bottom: 32 },
      tooltip: { trigger: "axis" },
      legend: { textStyle: { color: "#7b7d82" } },
      xAxis: { type: "category", data: labels, axisLabel: { show: false } },
      yAxis: {
        type: "value",
        axisLabel: { color: "#8a8c91" },
        splitLine: { lineStyle: { color: "#dededb" } },
      },
      series:
        mode === "live"
          ? [
              {
                name: "下载",
                type: "line",
                showSymbol: false,
                connectNulls: false,
                data: live.map((point) => point.download),
              },
              {
                name: "上传",
                type: "line",
                showSymbol: false,
                connectNulls: false,
                data: live.map((point) => point.upload),
              },
            ]
          : historyView === "speed"
            ? [
                {
                  name: "平均下载",
                  type: "line",
                  showSymbol: false,
                  data: history.map((point) => point.downloadRate),
                },
                {
                  name: "平均上传",
                  type: "line",
                  showSymbol: false,
                  data: history.map((point) => point.uploadRate),
                },
              ]
            : [
                {
                  name: "下载",
                  type: "bar",
                  stack: "bytes",
                  data: history.map((point) =>
                    Number(BigInt(point.download) / 1024n),
                  ),
                },
                {
                  name: "上传",
                  type: "bar",
                  stack: "bytes",
                  data: history.map((point) =>
                    Number(BigInt(point.upload) / 1024n),
                  ),
                },
                {
                  name: "累计",
                  type: "line",
                  yAxisIndex: 0,
                  showSymbol: false,
                  data: history.map((point) =>
                    Number(BigInt(point.cumulative) / 1024n),
                  ),
                },
              ],
    });
    const resize = () => chart.resize();
    window.addEventListener("resize", resize);
    return () => {
      window.removeEventListener("resize", resize);
      chart.dispose();
    };
  }, [history, historyView, live, mode]);
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
