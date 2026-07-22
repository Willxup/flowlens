import type { BreakdownBy, ByteString } from "../../api/contracts";
import { formatBytes, formatNetwork } from "../../lib/format";
import type { HistoricalTargetRow } from "./TargetList";

const labels: Record<BreakdownBy, string> = {
  target: "目标 IP",
  endpoint: "Endpoint",
  port: "端口",
  network: "TCP/UDP",
  source: "来源网段",
  domain: "域名",
};

export function BreakdownDistribution({
  by,
  rows,
}: {
  by: BreakdownBy;
  rows: HistoricalTargetRow[];
}) {
  const max = rows.reduce(
    (value, row) => maxBytes(value, row.totalBytes),
    "1" as ByteString,
  );
  return (
    <div className="breakdown-distribution" aria-label={`${labels[by]}分布`}>
      {rows.length === 0 ? (
        <p className="empty-state">当前范围没有可展示的数据。</p>
      ) : (
        rows.slice(0, 8).map((row) => (
          <div className="distribution-row" key={row.rawValue}>
            <div className="distribution-label">
              <div>
                <strong>{row.displayName}</strong>
                <span>
                  {row.rawValue}
                  {row.networkCode > 0
                    ? ` · ${formatNetwork(row.networkCode)}`
                    : ""}
                </span>
              </div>
              <strong>{formatBytes(row.totalBytes)}</strong>
            </div>
            <div className="distribution-track" aria-hidden="true">
              <i
                className="distribution-download"
                style={{ width: `${share(row.downloadBytes, max)}%` }}
              />
              <i
                className="distribution-upload"
                style={{ width: `${share(row.uploadBytes, max)}%` }}
              />
            </div>
            <div className="distribution-detail">
              <span>↓ {formatBytes(row.downloadBytes)}</span>
              <span>↑ {formatBytes(row.uploadBytes)}</span>
            </div>
          </div>
        ))
      )}
    </div>
  );
}

function maxBytes(left: ByteString, right: ByteString): ByteString {
  return BigInt(left) >= BigInt(right) ? left : right;
}

function share(value: ByteString, max: ByteString): number {
  return Number((BigInt(value) * 1000n) / BigInt(max)) / 10;
}
