import type { StorageResponse } from "../../api/contracts";
import { formatBytes } from "../../lib/format";

export function StoragePanel({ value }: { value: StorageResponse | null }) {
  if (value === null) return <p className="empty-state">存储状态正在加载。</p>;
  const cleanup = value.last_rollup_cleanup;
  const summary = value.protecting
    ? "数据库已进入容量保护，请检查空间和保留策略。"
    : cleanup === null
      ? "数据库空间正常，暂无聚合清理记录。"
      : cleanup.successful
        ? "数据库空间充足，最近一次聚合清理已经顺利完成。"
        : "数据库空间正常，但最近一次聚合清理失败。";
  return (
    <section className="panel storage-panel" aria-labelledby="storage-title">
      <div className="storage-lead">
        <span className="eyebrow">Storage health</span>
        <h2 id="storage-title">存储健康</h2>
        <p>{summary}</p>
      </div>
      <div className="storage-stat">
        <span>SQLite</span>
        <strong>{formatBytes(value.database_bytes)}</strong>
      </div>
      <div className="storage-stat">
        <span>WAL</span>
        <strong>{formatBytes(value.wal_bytes)}</strong>
      </div>
      <div className="storage-stat">
        <span>软上限</span>
        <strong>{formatBytes(value.soft_limit_bytes)}</strong>
      </div>
      <div className="storage-stat">
        <span>容量保护</span>
        <strong>{value.protecting ? "已启用" : "正常"}</strong>
      </div>
      <div className="storage-stat">
        <span>最近清理</span>
        <strong>
          {cleanup === null ? "暂无记录" : cleanup.successful ? "成功" : "失败"}
        </strong>
      </div>
    </section>
  );
}
