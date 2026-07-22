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

export function TargetList({
  live,
  liveTotalRate,
  historical,
}: {
  live?: LiveTargetResponse[];
  liveTotalRate?: number | null;
  historical?: HistoricalTargetRow[];
}) {
  const rows =
    live !== undefined
      ? live.map((item) => ({
          key: item.raw_endpoint,
          name: item.display_name,
          detail: `${item.raw_endpoint} · ${formatNetwork(item.network_code)} · ↓ ${formatRate(item.download_bytes_per_second)} · ↑ ${formatRate(item.upload_bytes_per_second)}${liveShare(item, liveTotalRate)}`,
          value: formatRate(
            item.upload_bytes_per_second + item.download_bytes_per_second,
          ),
          magnitude:
            item.upload_bytes_per_second + item.download_bytes_per_second,
        }))
      : (historical ?? []).map((item) => ({
          key: item.rawValue,
          name: item.displayName,
          detail: `${item.rawValue} · ${formatNetwork(item.networkCode)} · ↓ ${formatBytes(item.downloadBytes)} · ↑ ${formatBytes(item.uploadBytes)}`,
          value: formatBytes(item.totalBytes),
          magnitude: Number(
            BigInt(item.totalBytes) > 10_000_000_000n
              ? 10_000_000_000n
              : BigInt(item.totalBytes),
          ),
        }));
  const max = Math.max(1, ...rows.map((row) => row.magnitude));
  return (
    <div className="target-list">
      {rows.length === 0 ? (
        <p className="empty-state">当前没有可展示的目标。</p>
      ) : (
        rows.slice(0, 8).map((row, index) => (
          <div className="target-item" key={row.key}>
            <div
              className="target-icon target-rank"
              aria-label={`第 ${index + 1} 名`}
            >
              {index + 1}
            </div>
            <div className="target-main">
              <strong>{row.name}</strong>
              <span>{row.detail}</span>
              <div className="target-bar">
                <i
                  style={{
                    width: `${Math.max(4, (row.magnitude / max) * 100)}%`,
                  }}
                />
              </div>
            </div>
            <strong className="target-value">{row.value}</strong>
          </div>
        ))
      )}
    </div>
  );
}

function liveShare(
  item: LiveTargetResponse,
  totalRate: number | null | undefined,
): string {
  if (totalRate === undefined || totalRate === null || totalRate <= 0)
    return "";
  return ` · 占全局 ${formatRatio(
    (item.upload_bytes_per_second + item.download_bytes_per_second) / totalRate,
  )}`;
}
