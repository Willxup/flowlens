import { useMemo, useRef, useState, type PointerEvent } from "react";
import type { FlowLensDataSource } from "../../api/source";
import type { LiveChartPoint } from "../live/model";
import { TrafficChart } from "./TrafficChart";
import {
  buildTimeline,
  clampWindow,
  DAY_SECONDS,
  precisionLabel,
} from "./timeline";
import { useTimelineHistory } from "./useTimelineHistory";

export function LiveTrafficExplorer({
  source,
  live,
  onUnauthorized,
}: {
  source: FlowLensDataSource;
  live: LiveChartPoint[];
  onUnauthorized: () => void;
}) {
  const history = useTimelineHistory(source, onUnauthorized);
  const now = Math.floor(source.now().getTime() / 1000);
  const [selection, setSelection] = useState<[number, number] | null>(null);
  const bounds = useMemo<readonly [number, number]>(
    () => (selection ? clampWindow(selection, now) : [now - 3600, now]),
    [selection, now],
  );
  const timeline = useMemo(
    () => buildTimeline(history.data, live, now),
    [history.data, live, now],
  );
  const visible = useMemo(
    () =>
      timeline.points.filter(
        (point) => point.timestamp >= bounds[0] && point.timestamp <= bounds[1],
      ),
    [timeline.points, bounds],
  );
  const from = now - DAY_SECONDS;
  const percent = (value: number) => ((value - from) / DAY_SECONDS) * 100;
  const maximum = Math.max(
    1,
    ...timeline.preview.flatMap((point) => [
      point.download ?? 0,
      point.upload ?? 0,
    ]),
  );
  const trace = (key: "upload" | "download") => {
    let drawing = false;
    return timeline.preview
      .map((point) => {
        const value = point[key];
        if (value === null) {
          drawing = false;
          return "";
        }
        const segment = `${drawing ? "L" : "M"}${percent(point.timestamp) * 10},${58 - (value / maximum) * 48}`;
        drawing = true;
        return segment;
      })
      .join(" ");
  };
  const drag = useRef<{
    x: number;
    from: number;
    to: number;
    width: number;
    now: number;
  } | null>(null);
  function beginDrag(event: PointerEvent<SVGSVGElement>) {
    if (event.button !== 0) return;
    const box = event.currentTarget.getBoundingClientRect();
    const center =
      from + ((event.clientX - box.left) / box.width) * DAY_SECONDS;
    const width = bounds[1] - bounds[0];
    let start = bounds[0];
    if (center < bounds[0] || center > bounds[1])
      start = Math.max(from, Math.min(now - width, center - width / 2));
    setSelection([start, start + width]);
    drag.current = {
      x: event.clientX,
      from: start,
      to: start + width,
      width: box.width,
      now,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
  }
  function moveDrag(event: PointerEvent<SVGSVGElement>) {
    const origin = drag.current;
    if (!origin) return;
    const shift = ((event.clientX - origin.x) / origin.width) * DAY_SECONDS;
    const start = Math.max(
      origin.now - DAY_SECONDS,
      Math.min(origin.now - (origin.to - origin.from), origin.from + shift),
    );
    setSelection([start, start + origin.to - origin.from]);
  }
  const label = precisionLabel(visible);
  return (
    <section className="traffic-explorer" aria-label="实时与24小时流量图">
      <div className="timeline-toolbar">
        <span className="timeline-mode" role="status">
          {selection === null
            ? "跟随实时 · 最近 60 分钟"
            : "浏览历史 · 已暂停跟随"}
        </span>
        <div className="timeline-actions">
          <button type="button" onClick={() => setSelection([from, now])}>
            查看 24 小时
          </button>
          <button
            type="button"
            onClick={() => setSelection(null)}
            disabled={selection === null}
          >
            回到实时
          </button>
        </div>
      </div>
      <div className="timeline-range">
        <span>
          {timeLabel(bounds[0])} — {timeLabel(bounds[1])}
        </span>
        <span>{label}</span>
      </div>
      <TrafficChart mode="live" live={visible} liveBounds={bounds} />
      <div className="timeline-overview-head">
        <strong>最近 24 小时</strong>
        <span>拖动选区查看过去</span>
      </div>
      <div className="timeline-navigator">
        <svg
          viewBox="0 0 1000 68"
          preserveAspectRatio="none"
          aria-hidden="true"
          onPointerDown={beginDrag}
          onPointerMove={moveDrag}
          onPointerUp={() => {
            drag.current = null;
          }}
          onPointerCancel={() => {
            drag.current = null;
          }}
        >
          <path className="navigator-download" d={trace("download")} />
          <path className="navigator-upload" d={trace("upload")} />
          <rect
            className="navigator-mask"
            x="0"
            y="0"
            width={percent(bounds[0]) * 10}
            height="68"
          />
          <rect
            className="navigator-mask"
            x={percent(bounds[1]) * 10}
            y="0"
            width={(100 - percent(bounds[1])) * 10}
            height="68"
          />
          <rect
            className="navigator-selection"
            x={percent(bounds[0]) * 10}
            y="1"
            width={((bounds[1] - bounds[0]) / DAY_SECONDS) * 1000}
            height="66"
          />
        </svg>
      </div>
      <div className="timeline-axis">
        <span>{timeLabel(from)}</span>
        <span>{timeLabel(from + DAY_SECONDS / 2)}</span>
        <span>现在</span>
      </div>
      <div className="timeline-sliders">
        <label>
          <span>起点</span>
          <input
            type="range"
            aria-label="预览开始时间"
            aria-valuetext={timeLabel(bounds[0])}
            min={from}
            max={now - 60}
            step={60}
            value={bounds[0]}
            onChange={(event) =>
              setSelection(
                clampWindow([Number(event.target.value), bounds[1]], now),
              )
            }
          />
        </label>
        <label>
          <span>终点</span>
          <input
            type="range"
            aria-label="预览结束时间"
            aria-valuetext={timeLabel(bounds[1])}
            min={from + 60}
            max={now}
            step={60}
            value={bounds[1]}
            onChange={(event) => {
              const end = Number(event.target.value);
              setSelection(
                clampWindow([Math.min(bounds[0], end - 60), end], now),
              );
            }}
          />
        </label>
      </div>
      {history.error ? (
        <p className="timeline-notice" role="alert">
          24 小时历史加载失败；当前仅显示已取得的数据。
          <button type="button" onClick={history.retry}>
            重试
          </button>
        </p>
      ) : history.loading && !history.data ? (
        <p className="timeline-notice" role="status">
          正在加载 24 小时历史，实时样本仍正常显示。
        </p>
      ) : null}
      <p className="timeline-footnote">
        近 60
        分钟优先显示秒级样本，其余使用历史聚合；空缺表示未采集。预览按分钟归并。
      </p>
    </section>
  );
}
function timeLabel(timestamp: number) {
  return new Date(timestamp * 1000).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}
