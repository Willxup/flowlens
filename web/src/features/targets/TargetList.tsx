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
      ? live.map((item) => {
          const identity = targetIdentity(item.raw_endpoint, item.display_name);
          return {
            key: item.raw_endpoint,
            ...identity,
            network: formatNetwork(item.network_code),
            download: formatRate(item.download_bytes_per_second),
            upload: formatRate(item.upload_bytes_per_second),
            share: liveShare(item, liveTotalRate),
            value: formatRate(
              item.upload_bytes_per_second + item.download_bytes_per_second,
            ),
            magnitude:
              item.upload_bytes_per_second + item.download_bytes_per_second,
          };
        })
      : (historical ?? []).map((item) => {
          const identity = targetIdentity(item.rawValue, item.displayName);
          return {
            key: item.rawValue,
            ...identity,
            network: formatNetwork(item.networkCode),
            download: formatBytes(item.downloadBytes),
            upload: formatBytes(item.uploadBytes),
            share: null,
            value: formatBytes(item.totalBytes),
            magnitude: Number(
              BigInt(item.totalBytes) > 10_000_000_000n
                ? 10_000_000_000n
                : BigInt(item.totalBytes),
            ),
          };
        });
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
              <span className="target-detail">
                {row.rawDetail === null ? null : (
                  <>
                    <span>{row.rawDetail}</span>
                    <i aria-hidden="true">·</i>
                  </>
                )}
                <span>{row.network}</span>
                <i aria-hidden="true">·</i>
                <span aria-label={`下载 ${row.download}`}>
                  <b className="target-download" aria-hidden="true">
                    ↓
                  </b>{" "}
                  {row.download}
                </span>
                <i aria-hidden="true">·</i>
                <span aria-label={`上传 ${row.upload}`}>
                  <b className="target-upload" aria-hidden="true">
                    ↑
                  </b>{" "}
                  {row.upload}
                </span>
                {row.share === null ? null : (
                  <>
                    <i aria-hidden="true">·</i>
                    <span>{row.share}</span>
                  </>
                )}
              </span>
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

function targetIdentity(
  rawValue: string,
  displayName: string,
): { name: string; rawDetail: string | null } {
  const legacySuffix = ` · ${rawValue}`;
  const normalizedName = displayName.endsWith(legacySuffix)
    ? displayName.slice(0, -legacySuffix.length).trim()
    : displayName.trim();
  const name = normalizedName === "" ? rawValue : normalizedName;
  return { name, rawDetail: name === rawValue ? null : rawValue };
}

function liveShare(
  item: LiveTargetResponse,
  totalRate: number | null | undefined,
): string | null {
  if (totalRate === undefined || totalRate === null || totalRate <= 0)
    return null;
  return `占全局 ${formatRatio(
    (item.upload_bytes_per_second + item.download_bytes_per_second) / totalRate,
  )}`;
}
