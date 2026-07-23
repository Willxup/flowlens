import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
} from "react";
import type {
  BreakdownBy,
  LabelCandidateResponse,
  LabelResponse,
  RuntimeSessionResponse,
  StatusResponse,
  StorageResponse,
  TimeSelection,
} from "../../api/contracts";
import { UnauthorizedError } from "../../api/production";
import type { FlowLensDataSource } from "../../api/source";
import { Shell } from "../../app/Shell";
import { AliasDialog } from "../aliases/AliasDialog";
import { RangeSelector } from "../history/RangeSelector";
import { useHistoryViewModel } from "../history/useHistoryViewModel";
import { useLiveViewModel } from "../live/useLiveViewModel";
import { StoragePanel } from "../storage/StoragePanel";
import { TargetList, type HistoricalTargetRow } from "../targets/TargetList";
import { BreakdownDistribution } from "../targets/BreakdownDistribution";
import { Topology } from "../targets/Topology";
import { TrafficChart } from "../traffic/TrafficChart";
import { formatBytes, formatRate, formatRatio } from "../../lib/format";

const initialStatus: StatusResponse = {
  status: "degraded",
  reason: "starting",
  timezone: "UTC",
  auth_enabled: false,
  capabilities: {
    connection_id: false,
    source: false,
    destination: false,
    port: false,
    network: false,
    domain: false,
  },
};

export function DashboardPage({
  source,
  onUnauthorized,
}: {
  source: FlowLensDataSource;
  onUnauthorized: () => void;
}) {
  const [status, setStatus] = useState(initialStatus);
  const [selection, setSelection] = useState<TimeSelection>({ kind: "live" });
  const [by, setBy] = useState<BreakdownBy>("endpoint");
  const [historyChart, setHistoryChart] = useState<"traffic" | "speed">(
    "traffic",
  );
  const [storage, setStorage] = useState<StorageResponse | null>(null);
  const [labels, setLabels] = useState<LabelResponse[]>([]);
  const [candidates, setCandidates] = useState<LabelCandidateResponse[]>([]);
  const [sessions, setSessions] = useState<RuntimeSessionResponse[]>([]);
  const [aliasesOpen, setAliasesOpen] = useState(false);
  const [aliasRevision, setAliasRevision] = useState(0);
  const [logoutFailed, setLogoutFailed] = useState(false);
  const updateStatus = useCallback(
    (level: StatusResponse["status"], reason: string) => {
      setStatus((current) =>
        current.status === level && current.reason === reason
          ? current
          : { ...current, status: level, reason },
      );
    },
    [],
  );
  const reloadLabels = useCallback(async () => {
    setLabels(await source.labels());
    setAliasRevision((current) => current + 1);
  }, [source]);

  useEffect(() => {
    const controller = new AbortController();
    const handle = (error: unknown) => {
      if (error instanceof UnauthorizedError) onUnauthorized();
    };
    void source.status(controller.signal).then(setStatus).catch(handle);
    void source.storage(controller.signal).then(setStorage).catch(handle);
    void source.labels(controller.signal).then(setLabels).catch(handle);
    void source
      .labelCandidates(controller.signal)
      .then(setCandidates)
      .catch(handle);
    void source
      .runtimeSessions(controller.signal)
      .then(setSessions)
      .catch(handle);
    return () => controller.abort();
  }, [onUnauthorized, source]);

  const live = useLiveViewModel(
    source,
    status,
    selection.kind === "live",
    updateStatus,
    onUnauthorized,
  );
  const history = useHistoryViewModel(
    source,
    selection,
    status.timezone,
    by,
    onUnauthorized,
    aliasRevision,
  );
  const historicalRows = useMemo<HistoricalTargetRow[]>(
    () => history.targets?.items ?? [],
    [history.targets],
  );
  const topologyRows =
    selection.kind === "live"
      ? live.targets.map((item) => ({
          rawValue: item.raw_endpoint,
          displayName: item.display_name,
          networkCode: item.network_code,
          totalBytes: "0" as never,
          uploadBytes: "0" as never,
          downloadBytes: "0" as never,
        }))
      : historicalRows;

  async function logout() {
    setLogoutFailed(false);
    try {
      await source.logout();
      onUnauthorized();
    } catch (error) {
      if (error instanceof UnauthorizedError) {
        onUnauthorized();
        return;
      }
      setLogoutFailed(true);
    }
  }

  const liveMode = selection.kind === "live";
  const topologicalDimension = by === "target" || by === "endpoint";
  return (
    <Shell
      status={status.status}
      sourceMode={source.demo ? "demo" : "app"}
      authEnabled={source.demo || status.auth_enabled}
      onLogout={() => void logout()}
      logoutFailed={logoutFailed}
    >
      <section className="page-head">
        <div>
          <span className="eyebrow">Traffic overview</span>
          <h1 className="page-title">流量总览</h1>
        </div>
        <RangeSelector
          value={selection}
          now={source.now()}
          timezone={status.timezone}
          onChange={setSelection}
        />
      </section>
      <section
        className="dashboard"
        data-dashboard-mode={liveMode ? "live" : "history"}
      >
        <article className="panel hero-panel">
          <header className="panel-head">
            <div>
              <span className="eyebrow">
                {liveMode ? "Live throughput" : "Historical traffic"}
              </span>
              <h2>{liveMode ? "实时吞吐" : "历史流量"}</h2>
              <p>
                {liveMode
                  ? "最近 60 分钟 · 1 秒采样"
                  : "SQLite 聚合 · 自动选择分辨率"}
              </p>
            </div>
            <span className="micro">
              {liveMode
                ? live.connected
                  ? `${formatInterval(live.intervalMillis)}采样 · SSE 已连接`
                  : "SSE 重连中"
                : history.loading
                  ? "正在加载"
                  : history.error
                    ? "历史数据加载失败"
                    : "按范围查询"}
            </span>
          </header>
          <div className="chart-summary">
            <Metric
              label={liveMode ? "当前下载" : "下载总量"}
              value={
                liveMode
                  ? formatRate(live.currentDownload)
                  : history.view
                    ? formatBytes(history.view.downloadBytes)
                    : "—"
              }
            />
            <Metric
              label={liveMode ? "当前上传" : "上传总量"}
              value={
                liveMode
                  ? formatRate(live.currentUpload)
                  : history.view
                    ? formatBytes(history.view.uploadBytes)
                    : "—"
              }
            />
            <Metric
              label={liveMode ? "活动连接" : "总流量"}
              value={
                liveMode
                  ? String(live.activeConnections ?? "—")
                  : history.view
                    ? formatBytes(history.view.totalBytes)
                    : "—"
              }
            />
          </div>
          <div className="metric-grid">
            {liveMode ? (
              <>
                <Metric
                  label="1 分钟平均下载"
                  value={formatRate(live.averageDownload1m)}
                />
                <Metric
                  label="1 分钟平均上传"
                  value={formatRate(live.averageUpload1m)}
                />
                <Metric
                  label="5 分钟平均下载"
                  value={formatRate(live.averageDownload5m)}
                />
                <Metric
                  label="5 分钟平均上传"
                  value={formatRate(live.averageUpload5m)}
                />
                <Metric
                  label="60 分钟峰值下载"
                  value={formatRate(live.peakDownload60m)}
                />
                <Metric
                  label="60 分钟峰值上传"
                  value={formatRate(live.peakUpload60m)}
                />
              </>
            ) : (
              <>
                <Metric
                  label="较上一周期"
                  value={comparisonText(
                    history.view?.totalBytes,
                    history.view?.previousBytes,
                    selection,
                  )}
                />
                <Metric
                  label="平均下载"
                  value={formatRate(history.view?.averageDownload ?? null)}
                />
                <Metric
                  label="平均上传"
                  value={formatRate(history.view?.averageUpload ?? null)}
                />
                <Metric
                  label="峰值下载"
                  value={formatRate(history.view?.peakDownload ?? null)}
                />
                <Metric
                  label="峰值上传"
                  value={formatRate(history.view?.peakUpload ?? null)}
                />
                <Metric
                  label="平均连接"
                  value={formatCount(history.view?.averageConnections ?? null)}
                />
                <Metric
                  label="峰值连接"
                  value={formatCount(history.view?.peakConnections ?? null)}
                />
                <Metric
                  label="数据完整率"
                  value={formatRatio(history.view?.completeness ?? null)}
                />
              </>
            )}
          </div>
          {!liveMode ? (
            <div className="chart-toggle" aria-label="历史图表视图">
              <button
                type="button"
                aria-pressed={historyChart === "traffic"}
                onClick={() => setHistoryChart("traffic")}
              >
                流量视图
              </button>
              <button
                type="button"
                aria-pressed={historyChart === "speed"}
                onClick={() => setHistoryChart("speed")}
              >
                速度视图
              </button>
            </div>
          ) : null}
          <TrafficChart
            mode={liveMode ? "live" : "history"}
            live={live.chart}
            history={history.view?.chart}
            historyView={historyChart}
          />
        </article>

        <article className="panel topology-panel">
          <header className="panel-head">
            <div>
              <span className="eyebrow">Traffic topology</span>
              <h2>{liveMode ? "实时目标分析" : `${dimensionLabel(by)}分析`}</h2>
              <p>{liveMode ? "当前目标速率" : "所选周期累计流量"} · 近似归因</p>
            </div>
            {!liveMode ? <DimensionSelect value={by} onChange={setBy} /> : null}
          </header>
          {liveMode || topologicalDimension ? (
            <Topology targets={topologyRows} />
          ) : (
            <BreakdownDistribution by={by} rows={historicalRows} />
          )}
        </article>

        <ConfidencePanel
          liveMode={liveMode}
          connectionCoverage={
            liveMode
              ? live.connectionCoverage
              : (history.targets?.connectionCoverage ?? null)
          }
          dimensionRetention={
            liveMode ? null : (history.targets?.dimensionRetention ?? null)
          }
          unattributed={
            liveMode || !history.targets
              ? null
              : formatBytes(history.targets.unattributedBytes)
          }
          other={
            liveMode || !history.targets
              ? null
              : formatBytes(history.targets.otherBytes)
          }
          statusReason={status.reason}
          observedAt={liveMode ? live.observedAt : null}
          intervalMillis={liveMode ? live.intervalMillis : null}
          completeness={liveMode ? null : (history.view?.completeness ?? null)}
          recovered={
            liveMode || !history.view
              ? null
              : formatBytes(history.view.recoveredBytes)
          }
          resetCount={liveMode ? null : (history.view?.resetCount ?? null)}
          qualityEventCount={
            liveMode ? null : (history.view?.qualityEvents.length ?? null)
          }
          boundaryApproximate={
            liveMode ? null : (history.view?.boundaryApproximate ?? null)
          }
          sessions={liveMode ? [] : sessions}
        />

        <article className="panel targets-panel">
          <header className="panel-head">
            <div>
              <span className="eyebrow">Observed targets</span>
              <h2>{liveMode ? "当前目标排行" : "历史维度排行"}</h2>
              <p>
                {liveMode
                  ? "按当前速率排序，别名后保留原始端点。"
                  : "按所选周期累计流量排序，结果为近似归因。"}
              </p>
            </div>
            <button
              className="icon-button"
              type="button"
              aria-label="管理别名"
              onClick={() => setAliasesOpen(true)}
            >
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="m4 16-.8 4.8L8 20l10.6-10.6a2 2 0 0 0-2.8-2.8L5.2 17.2M14.5 8l2.8 2.8" />
              </svg>
            </button>
          </header>
          {liveMode ? (
            <TargetList
              live={live.targets}
              liveTotalRate={live.targetGlobalRate}
            />
          ) : (
            <TargetList historical={historicalRows} />
          )}
        </article>

        <StoragePanel value={storage} />
      </section>
      {aliasesOpen ? (
        <AliasDialog
          source={source}
          labels={labels}
          candidates={candidates}
          onChanged={reloadLabels}
          onClose={() => setAliasesOpen(false)}
        />
      ) : null}
    </Shell>
  );
}

function ConfidencePanel({
  liveMode,
  connectionCoverage,
  dimensionRetention,
  unattributed,
  other,
  statusReason,
  observedAt,
  intervalMillis,
  completeness,
  recovered,
  resetCount,
  qualityEventCount,
  boundaryApproximate,
  sessions,
}: {
  liveMode: boolean;
  connectionCoverage: number | null;
  dimensionRetention: number | null;
  unattributed: string | null;
  other: string | null;
  statusReason: string;
  observedAt: number | null;
  intervalMillis: number | null;
  completeness: number | null;
  recovered: string | null;
  resetCount: number | null;
  qualityEventCount: number | null;
  boundaryApproximate: boolean | null;
  sessions: RuntimeSessionResponse[];
}) {
  return (
    <aside className="panel confidence-panel">
      <span className="eyebrow">Data confidence</span>
      <h2>数据质量</h2>
      <p className="confidence-intro">
        全局总量精确；下面说明目标排行能解释其中多少流量。
      </p>
      <div className="coverage-list">
        <CoverageRow
          label="可归因覆盖"
          value={connectionCoverage}
          detail="全局流量中，能关联到活动连接维度的部分。"
        />
        {!liveMode ? (
          <CoverageRow
            label="排行覆盖"
            value={dimensionRetention}
            detail="已归因流量中，仍保留在当前 Top K 排行里的部分。"
          />
        ) : null}
      </div>
      <dl className="confidence-stats">
        <div>
          <dt>未归因流量</dt>
          <dd>{liveMode ? "实时不累计" : (unattributed ?? "—")}</dd>
        </div>
        {!liveMode ? (
          <div>
            <dt>Top K 之外</dt>
            <dd>{other ?? "—"}</dd>
          </div>
        ) : null}
        <div>
          <dt>采集状态</dt>
          <dd>{statusReason}</dd>
        </div>
        {liveMode ? (
          <>
            <div>
              <dt>最后观测</dt>
              <dd>
                {observedAt === null
                  ? "—"
                  : new Date(observedAt * 1000).toLocaleTimeString("zh-CN", {
                      hour: "2-digit",
                      minute: "2-digit",
                      second: "2-digit",
                    })}
              </dd>
            </div>
            <div>
              <dt>实际间隔</dt>
              <dd>{formatInterval(intervalMillis)}</dd>
            </div>
          </>
        ) : (
          <>
            <div>
              <dt>数据完整率</dt>
              <dd>{formatRatio(completeness)}</dd>
            </div>
            <div>
              <dt>恢复流量</dt>
              <dd>{recovered ?? "—"}</dd>
            </div>
            <div>
              <dt>重置次数</dt>
              <dd>{resetCount ?? "—"}</dd>
            </div>
            <div>
              <dt>质量事件</dt>
              <dd>{qualityEventCount ?? "—"}</dd>
            </div>
            <div>
              <dt>边界估算</dt>
              <dd>
                {boundaryApproximate === null
                  ? "—"
                  : boundaryApproximate
                    ? "已近似"
                    : "精确边界"}
              </dd>
            </div>
          </>
        )}
      </dl>
      {!liveMode ? <RuntimeSessions sessions={sessions} /> : null}
    </aside>
  );
}

function RuntimeSessions({ sessions }: { sessions: RuntimeSessionResponse[] }) {
  return (
    <section className="runtime-sessions" aria-label="最近运行">
      <div className="runtime-sessions-head">
        <strong>最近运行</strong>
        <span>采集进程上下文</span>
      </div>
      {sessions.length === 0 ? (
        <p className="empty-state">暂无运行记录。</p>
      ) : (
        <div className="runtime-session-list">
          {sessions.slice(0, 3).map((session) => (
            <article
              className="runtime-session"
              key={`${session.started_at}-${session.last_seen_at}`}
            >
              <div>
                <strong>
                  {session.ended_at === null ? "当前运行" : "历史运行"}
                </strong>
                <span>{session.sing_box_version}</span>
              </div>
              <span>
                {new Date(session.started_at * 1000).toLocaleString("zh-CN", {
                  month: "2-digit",
                  day: "2-digit",
                  hour: "2-digit",
                  minute: "2-digit",
                })}
                {session.ended_at === null ? " 至今" : " 已结束"}
              </span>
              <span>
                {session.data_gap_before_seconds > 0
                  ? `前序缺口 ${session.data_gap_before_seconds} 秒`
                  : "连续采集"}
              </span>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}

function CoverageRow({
  label,
  value,
  detail,
}: {
  label: string;
  value: number | null;
  detail: string;
}) {
  const percent = value === null ? null : Math.round(value * 1000) / 10;
  return (
    <div className="coverage-row">
      <div className="coverage-label">
        <strong>{label}</strong>
        <span>{formatRatio(value)}</span>
      </div>
      <div
        className="coverage-track"
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent ?? undefined}
      >
        <i
          style={
            {
              "--coverage": `${percent === null ? 0 : percent}%`,
            } as CSSProperties
          }
        />
      </div>
      <p>{detail}</p>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="chart-value">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

const dimensions: Array<{ value: BreakdownBy; label: string }> = [
  { value: "target", label: "目标 IP" },
  { value: "endpoint", label: "Endpoint" },
  { value: "port", label: "端口" },
  { value: "network", label: "TCP/UDP" },
  { value: "source", label: "来源网段" },
  { value: "domain", label: "域名" },
];

function dimensionLabel(value: BreakdownBy): string {
  return dimensions.find((dimension) => dimension.value === value)!.label;
}

function DimensionSelect({
  value,
  onChange,
}: {
  value: BreakdownBy;
  onChange: (value: BreakdownBy) => void;
}) {
  return (
    <div className="dimension-tabs" aria-label="分析维度">
      {dimensions.map((dimension) => (
        <button
          key={dimension.value}
          type="button"
          aria-pressed={value === dimension.value}
          onClick={() => onChange(dimension.value)}
        >
          {dimension.label}
        </button>
      ))}
    </div>
  );
}

function formatCount(value: number | null): string {
  return value === null ? "—" : value.toFixed(value % 1 === 0 ? 0 : 1);
}

function formatInterval(value: number | null): string {
  return value === null ? "" : `${(value / 1000).toFixed(1)} 秒`;
}

function comparisonText(
  current: string | undefined,
  previous: string | undefined,
  selection: TimeSelection,
): string {
  if (selection.kind === "preset" && selection.preset === "lifetime")
    return "不适用";
  if (current === undefined || previous === undefined) return "—";
  const prior = BigInt(previous);
  if (prior === 0n) return "首次统计";
  const tenths = Number(((BigInt(current) - prior) * 1000n) / prior) / 10;
  return `${tenths >= 0 ? "+" : ""}${tenths.toFixed(1)}%`;
}
