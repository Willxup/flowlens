import { useMemo, useState } from "react";
import type { ByteString, LiveTargetResponse } from "../../api/contracts";
import {
  formatBytes,
  formatNetwork,
  formatRate,
  formatRatio,
} from "../../lib/format";

export type HistoricalTargetRow = {
  rawValue: string;
  displayName: string;
  networkCode: number;
  totalBytes: ByteString;
  uploadBytes: ByteString;
  downloadBytes: ByteString;
};

type Row = {
  key: string;
  name: string;
  raw: string;
  network: string;
  download: string;
  upload: string;
  value: string;
  share: string;
  weight: bigint;
};

export function TargetList({
  live,
  liveTotalRate,
  historical,
  historicalGlobalBytes,
  available = true,
  noTraffic = false,
  loading = false,
  error = false,
}: {
  live?: LiveTargetResponse[];
  liveTotalRate?: number | null;
  historical?: HistoricalTargetRow[];
  historicalGlobalBytes?: ByteString;
  available?: boolean;
  noTraffic?: boolean;
  loading?: boolean;
  error?: boolean;
}) {
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<"traffic" | "name">("traffic");
  const rows = useMemo<Row[]>(() => {
    const result =
      live !== undefined
        ? live.map((item) => {
            const weight =
              item.upload_bytes_per_second + item.download_bytes_per_second;
            return {
              key: item.raw_endpoint,
              name: targetName(item.raw_endpoint, item.display_name),
              raw: item.raw_endpoint,
              network: formatNetwork(item.network_code),
              download: formatRate(item.download_bytes_per_second),
              upload: formatRate(item.upload_bytes_per_second),
              value: formatRate(weight),
              share: liveShareText(weight, liveTotalRate),
              weight: BigInt(Math.round(weight * 1000)),
            };
          })
        : (historical ?? []).map((item) => ({
            key: item.rawValue,
            name: targetName(item.rawValue, item.displayName),
            raw: item.rawValue,
            network: formatNetwork(item.networkCode),
            download: formatBytes(item.downloadBytes),
            upload: formatBytes(item.uploadBytes),
            value: formatBytes(item.totalBytes),
            share: historicalShareText(item.totalBytes, historicalGlobalBytes),
            weight: BigInt(item.totalBytes),
          }));
    return result
      .filter((row) =>
        (row.name + " " + row.raw + " " + row.network)
          .toLocaleLowerCase()
          .includes(query.trim().toLocaleLowerCase()),
      )
      .sort((a, b) =>
        sort === "name"
          ? a.name.localeCompare(b.name, "zh-CN")
          : a.weight === b.weight
            ? a.raw.localeCompare(b.raw)
            : a.weight > b.weight
              ? -1
              : 1,
      );
  }, [historical, historicalGlobalBytes, live, liveTotalRate, query, sort]);
  const max = rows.reduce(
    (value, row) => (row.weight > value ? row.weight : value),
    1n,
  );
  const empty = loading
    ? "目标数据正在加载。"
    : error
      ? "目标数据暂时无法加载。"
      : !available
        ? "当前采集能力不支持这个维度。"
        : noTraffic
          ? "所选时间没有流量。"
          : query
            ? "没有匹配的目标。"
            : "当前没有可展示的目标。";
  return (
    <section className="target-explorer" aria-label="目标列表">
      <div className="explorer-controls">
        <label className="search-field">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="10.8" cy="10.8" r="6.4" />
            <path d="m16 16 4.2 4.2" />
          </svg>
          <span className="sr-only">搜索目标</span>
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索名称、端点或协议"
            aria-label="搜索目标"
          />
        </label>
        <div className="sort-control" aria-label="目标排序">
          <button
            type="button"
            aria-pressed={sort === "traffic"}
            onClick={() => setSort("traffic")}
          >
            按流量
          </button>
          <button
            type="button"
            aria-pressed={sort === "name"}
            onClick={() => setSort("name")}
          >
            按名称
          </button>
        </div>
      </div>
      <div className="explorer-summary">
        <span>{loading ? "正在读取目标" : `显示 ${rows.length} 个目标`}</span>
        <span>
          {live !== undefined ? "当前速率 · 近似归因" : "周期累计 · 近似归因"}
        </span>
      </div>
      {rows.length === 0 ? (
        <p className="empty-state" role="status">
          {empty}
        </p>
      ) : (
        <div className="target-list">
          {rows.map((row, index) => (
            <article className="target-item" key={row.key}>
              <span className="target-rank" aria-label={`第 ${index + 1} 名`}>
                {String(index + 1).padStart(2, "0")}
              </span>
              <div className="target-main">
                <div className="target-heading">
                  <strong title={row.name}>{row.name}</strong>
                  <span className="protocol-chip">{row.network}</span>
                </div>
                {row.name === row.raw ? null : (
                  <span className="target-raw" title={row.raw}>
                    {row.raw}
                  </span>
                )}
                <div className="target-bar" aria-hidden="true">
                  <i
                    style={{
                      width: `${row.weight <= 0n ? 0 : Math.max(1, Number((row.weight * 1000n) / max) / 10)}%`,
                    }}
                  />
                </div>
              </div>
              <div className="target-values">
                <strong>{row.value}</strong>
                <span>{row.share}</span>
                <small>
                  <span aria-label={`下载 ${row.download}`}>
                    <b className="target-download" aria-hidden="true">
                      ↓
                    </b>{" "}
                    {row.download}
                  </span>
                  <span aria-label={`上传 ${row.upload}`}>
                    <b className="target-upload" aria-hidden="true">
                      ↑
                    </b>{" "}
                    {row.upload}
                  </span>
                </small>
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}

function targetName(raw: string, display: string): string {
  const suffix = ` · ${raw}`;
  const normalized = display.endsWith(suffix)
    ? display.slice(0, -suffix.length)
    : display;
  return normalized.trim() || raw;
}

function liveShareText(
  weight: number,
  globalRate: number | null | undefined,
): string {
  if (globalRate === null || globalRate === undefined) return "占比未知";
  if (globalRate === 0) return "全局速率为 0";
  const ratio = weight / globalRate;
  return ratio > 1 ? "估算超出全局" : `占全局 ${formatRatio(ratio)}`;
}

function historicalShareText(
  total: ByteString,
  global: ByteString | undefined,
): string {
  if (global === undefined) return "占比未知";
  const denominator = BigInt(global);
  if (denominator === 0n) return "全局流量为 0";
  if (BigInt(total) > denominator) return "估算超出全局";
  const ratio = Number((BigInt(total) * 10000n) / denominator) / 10000;
  return `占全局 ${formatRatio(ratio)}`;
}
