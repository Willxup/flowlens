import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useLayoutEffect,
  useState,
  type CSSProperties,
} from "react";
import type {
  BreakdownBy,
  ByteString,
  LabelCandidateResponse,
  LabelResponse,
  RuntimeSessionResponse,
  StatusResponse,
  StorageResponse,
  TimeSelection,
} from "../../api/contracts";
import { UnauthorizedError } from "../../api/production";
import type { FlowLensDataSource } from "../../api/source";
import { Shell, type Workspace } from "../../app/Shell";
import { InfoTooltip } from "../../components/Tooltip";
import { formatBytes, formatRate, formatRatio } from "../../lib/format";
import { AliasDialog } from "../aliases/AliasDialog";
import { RangeSelector } from "../history/RangeSelector";
import { useHistoryViewModel } from "../history/useHistoryViewModel";
import { useLiveViewModel } from "../live/useLiveViewModel";
import { StoragePanel } from "../storage/StoragePanel";
import { FlowMap } from "../targets/FlowMap";
import { TargetList, type HistoricalTargetRow } from "../targets/TargetList";
import { TrafficChart } from "../traffic/TrafficChart";

const initialStatus: StatusResponse = {
  status: "degraded",
  reason: "starting",
  timezone: "UTC",
  version: "",
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
const dimensions: Array<{ value: BreakdownBy; label: string }> = [
  { value: "target", label: "目标 IP" },
  { value: "endpoint", label: "Endpoint" },
  { value: "port", label: "端口" },
  { value: "network", label: "TCP/UDP" },
  { value: "source", label: "来源网段" },
  { value: "domain", label: "域名" },
];

export function DashboardPage({
  source,
  onUnauthorized,
}: {
  source: FlowLensDataSource;
  onUnauthorized: () => void;
}) {
  const [workspace, setWorkspace] = useState<Workspace>("overview");
  const [status, setStatus] = useState(initialStatus);
  const [selection, setSelection] = useState<TimeSelection>({ kind: "live" });
  const [by, setBy] = useState<BreakdownBy>("endpoint");
  const [historyChart, setHistoryChart] = useState<"traffic" | "speed">(
    "speed",
  );
  const [storage, setStorage] = useState<StorageResponse | null>(null);
  const [storageError, setStorageError] = useState(false);
  const [labels, setLabels] = useState<LabelResponse[]>([]);
  const [candidates, setCandidates] = useState<LabelCandidateResponse[]>([]);
  const [sessions, setSessions] = useState<RuntimeSessionResponse[]>([]);
  const [sessionsError, setSessionsError] = useState(false);
  const [aliasesOpen, setAliasesOpen] = useState(false);
  const [aliasRevision, setAliasRevision] = useState(0);
  const [logoutFailed, setLogoutFailed] = useState(false);
  const [, refreshClock] = useState(0);
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
    void source
      .storage(controller.signal)
      .then(setStorage)
      .catch((error) => {
        setStorageError(true);
        handle(error);
      });
    void source.labels(controller.signal).then(setLabels).catch(handle);
    void source
      .labelCandidates(controller.signal)
      .then(setCandidates)
      .catch(handle);
    void source
      .runtimeSessions(controller.signal)
      .then(setSessions)
      .catch((error) => {
        setSessionsError(true);
        handle(error);
      });
    return () => controller.abort();
  }, [onUnauthorized, source]);

  useEffect(() => {
    if (selection.kind !== "live") return;
    const interval = window.setInterval(
      () => refreshClock((value) => value + 1),
      5_000,
    );
    return () => window.clearInterval(interval);
  }, [selection.kind]);

  const liveMode = selection.kind === "live";
  const live = useLiveViewModel(
    source,
    status,
    workspace === "overview" || (workspace !== "history" && liveMode),
    updateStatus,
    onUnauthorized,
  );
  const history = useHistoryViewModel(
    source,
    selection,
    by,
    onUnauthorized,
    aliasRevision,
  );
  const historicalRows = useMemo<HistoricalTargetRow[]>(
    () => history.targets?.items ?? [],
    [history.targets],
  );
  const clock = source.now().getTime() / 1000;
  const sampleStale =
    live.sampledAt === null || !live.connected || clock - live.sampledAt > 15;
  const targetStale =
    live.observedAt !== null &&
    (clock - live.observedAt >
      Math.max(30, ((live.intervalMillis ?? 0) / 1000) * 3) ||
      (live.targetsUpdatedAt !== null &&
        Date.now() - live.targetsUpdatedAt > 30_000));
  const targetState = live.targetError
    ? live.observedAt === null
      ? "error"
      : "stale"
    : live.targetLoading && live.observedAt === null
      ? "loading"
      : targetStale
        ? "stale"
        : "fresh";
  const targetSummary =
    targetState === "fresh"
      ? `目标快照 ${formatClock(live.observedAt)} · 约每 10 秒更新`
      : targetState === "loading"
        ? "正在读取目标快照"
        : targetState === "stale"
          ? `目标快照已过期 · 最后观测 ${formatClock(live.observedAt)}`
          : "目标快照读取失败";

  const scrollPositions = useRef<Partial<Record<Workspace, number>>>({});
  const previousWorkspace = useRef(workspace);
  useLayoutEffect(() => {
    if (previousWorkspace.current === workspace) return;
    previousWorkspace.current = workspace;
    window.scrollTo(0, scrollPositions.current[workspace] ?? 0);
    document
      .querySelector<HTMLElement>(".page-title")
      ?.focus({ preventScroll: true });
  }, [workspace]);
  function navigate(next: Workspace) {
    if (next === workspace) return;
    scrollPositions.current[workspace] = window.scrollY;
    setWorkspace(next);
    if (next === "overview") setSelection({ kind: "live" });
    if (next === "history" && selection.kind === "live")
      setSelection({ kind: "preset", preset: "today" });
  }
  function selectRange(next: TimeSelection) {
    setSelection(next);
    if (next.kind === "live" && workspace === "history")
      setWorkspace("overview");
  }
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
  const commonShell = {
    status: status.status,
    version: status.version,
    sourceMode: source.demo ? ("demo" as const) : ("app" as const),
    authEnabled: source.demo || status.auth_enabled,
    onLogout: () => void logout(),
    logoutFailed,
    workspace,
    onNavigate: navigate,
  };
  return (
    <Shell {...commonShell}>
      <section className="workspace-head">
        <div>
          <span className="eyebrow">
            {workspace === "overview"
              ? "LIVE / NETWORK SIGNAL"
              : workspace === "targets"
                ? "EXPLORE / DESTINATIONS"
                : workspace === "history"
                  ? "ANALYZE / TIMELINE"
                  : "TRUST / DATA INTEGRITY"}
          </span>
          <h1 className="page-title" tabIndex={-1}>
            {workspace === "overview"
              ? "实时总览"
              : workspace === "targets"
                ? "目标探索"
                : workspace === "history"
                  ? "历史分析"
                  : "质量与存储"}
          </h1>
          <p className="workspace-intro">
            {workspace === "overview"
              ? "沿着每一次采样，观察流量的方向与变化。"
              : workspace === "targets"
                ? "从全局流量走向具体端点，查看可归因的连接。"
                : workspace === "history"
                  ? "把速率、累计流量与采集质量放在同一条时间线上。"
                  : "理解采集状态、归因边界与本地存储健康。"}
          </p>
        </div>
        {workspace === "overview" ? (
          <div className="head-live-label">
            <span className={`signal-dot ${sampleStale ? "stale" : ""}`} />
            <div>
              <strong>{sampleStale ? "实时信号中断" : "实时信号已连接"}</strong>
              <small>
                {sampleStale ? "显示最后观测值" : "最近 60 分钟 · 逐秒采样"}
              </small>
            </div>
          </div>
        ) : (
          <RangeSelector
            value={selection}
            now={source.now()}
            timezone={status.timezone}
            onChange={selectRange}
          />
        )}
      </section>

      {workspace === "overview" ? (
        <div className="overview-workspace workspace-enter">
          <section className="signal-stage" aria-labelledby="signal-title">
            <div className="section-heading stage-heading">
              <div>
                <span className="eyebrow">SIGNAL / 00</span>
                <h2 id="signal-title">当前吞吐</h2>
              </div>
              <span className="micro">
                {sampleStale
                  ? `最后采样 ${formatClock(live.sampledAt)} · 数据已停止更新`
                  : "SSE 实时连接"}
              </span>
            </div>
            <div className="signal-main">
              <div className="signal-numbers">
                <div className="signal-number download">
                  <span>
                    <i />
                    {sampleStale ? "最后下载速率" : "当前下载速率"}
                  </span>
                  <strong>{formatRate(live.currentDownload)}</strong>
                  <small>DOWNLOAD / INBOUND</small>
                </div>
                <div className="signal-number upload">
                  <span>
                    <i />
                    {sampleStale ? "最后上传速率" : "当前上传速率"}
                  </span>
                  <strong>{formatRate(live.currentUpload)}</strong>
                  <small>UPLOAD / OUTBOUND</small>
                </div>
                <div className="signal-connections">
                  <span>活动连接</span>
                  <strong>
                    {live.activeConnections === null
                      ? "—"
                      : live.activeConnections.toLocaleString("zh-CN")}
                  </strong>
                  <small>{targetSummary}</small>
                </div>
              </div>
              <div className="signal-chart">
                <div className="chart-caption">
                  <span>THROUGHPUT / LAST 60 MIN</span>
                </div>
                <TrafficChart mode="live" live={live.chart} />
              </div>
            </div>
            <div className="stage-metrics">
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
            </div>
          </section>
          <div className="overview-lower">
            <FlowMap
              targets={live.targets}
              globalRate={live.targetGlobalRate}
              coverage={live.connectionCoverage}
              state={targetState}
            />
            <aside className="overview-aside">
              <div className="section-heading">
                <div>
                  <span className="eyebrow">READOUT / 02</span>
                  <h2>观测摘要</h2>
                </div>
              </div>
              <div className="readout-row">
                <span>流量记录</span>
                <strong>全局精确</strong>
              </div>
              <div className="readout-row">
                <span>目标归因</span>
                <strong>采样近似</strong>
              </div>
              <div className="readout-row">
                <span>可归因覆盖</span>
                <strong>{formatRatio(live.connectionCoverage)}</strong>
              </div>
              <div className="readout-row">
                <span>目标快照间隔</span>
                <strong>{formatInterval(live.intervalMillis)}</strong>
              </div>
              <p className="readout-note">
                {targetSummary}
                。目标排行和全局吞吐来自不同采样节奏，不能相互当作精确拆分。
              </p>
              <button
                className="text-link"
                type="button"
                onClick={() => navigate("targets")}
              >
                探索全部目标 <span aria-hidden="true">↗</span>
              </button>
            </aside>
          </div>
        </div>
      ) : null}

      {workspace === "targets" ? (
        <div className="targets-workspace workspace-enter">
          <div className="section-heading">
            <div>
              <span className="eyebrow">RANKED SIGNAL / 01</span>
              <h2>{liveMode ? "当前目标速率" : "历史维度排行"}</h2>
            </div>
            <button
              className="outline-button"
              type="button"
              onClick={() => setAliasesOpen(true)}
            >
              管理别名 <span aria-hidden="true">↗</span>
            </button>
          </div>
          {liveMode ? (
            <div className={`data-banner ${targetState}`} role="status">
              <span>{targetSummary}</span>
              <span>
                全局速率 {formatRate(live.targetGlobalRate)} · 可归因覆盖{" "}
                {formatRatio(live.connectionCoverage)}
              </span>
            </div>
          ) : (
            <>
              <DimensionSelect value={by} onChange={setBy} />
              <div
                className={`data-banner ${history.error ? "error" : ""}`}
                role="status"
              >
                <span>
                  {history.error
                    ? "历史数据加载失败；当前显示上次结果"
                    : history.loading
                      ? "正在查询目标"
                      : "所选周期 · 近似归因"}
                </span>
                <span>
                  全局累计 {displayBytes(history.targets?.globalBytes)} ·
                  排行覆盖{" "}
                  {formatRatio(history.targets?.dimensionRetention ?? null)}
                </span>
              </div>
            </>
          )}
          <TargetList
            live={liveMode ? live.targets : undefined}
            liveTotalRate={liveMode ? live.targetGlobalRate : undefined}
            historical={liveMode ? undefined : historicalRows}
            historicalGlobalBytes={history.targets?.globalBytes}
            available={liveMode || (history.targets?.available ?? true)}
            noTraffic={!liveMode && (history.targets?.noTraffic ?? false)}
            loading={
              liveMode
                ? live.targetLoading
                : history.loading && history.targets === null
            }
            error={
              liveMode
                ? targetState === "error"
                : history.error && history.targets === null
            }
          />
        </div>
      ) : null}

      {workspace === "history" ? (
        <div
          className="history-workspace workspace-enter"
          aria-busy={history.loading}
        >
          <section className="history-stage">
            <div className="section-heading">
              <div>
                <span className="eyebrow">TRAFFIC / 01</span>
                <div className="heading-with-tooltip">
                  <h2>历史流量</h2>
                  <InfoTooltip
                    content="SQLite 聚合 · 自动选择分辨率"
                    label="查看“历史流量”说明"
                  />
                </div>
              </div>
              <span className="micro">
                {history.loading
                  ? history.view
                    ? "正在更新 · 暂显示上次结果"
                    : "正在加载"
                  : history.error
                    ? "查询失败 · 保留上次结果"
                    : "按范围查询"}
              </span>
            </div>
            {history.error ? (
              <p className="data-alert" role="alert">
                历史查询失败，当前图表可能是上一次成功查询的结果。
              </p>
            ) : null}
            <div className="history-totals">
              <Metric
                label="下载总量"
                value={displayBytes(history.view?.downloadBytes)}
                accent="download"
              />
              <Metric
                label="上传总量"
                value={displayBytes(history.view?.uploadBytes)}
                accent="upload"
              />
              <Metric
                label="总流量"
                value={displayBytes(history.view?.totalBytes)}
              />
              <Metric
                label="较上一周期"
                value={comparisonText(
                  history.view?.totalBytes,
                  history.view?.previousBytes,
                  selection,
                )}
              />
            </div>
            <div className="chart-toolbar">
              <span>TRAFFIC TIMELINE</span>
              <div className="chart-toggle" aria-label="历史图表视图">
                <button
                  type="button"
                  aria-pressed={historyChart === "speed"}
                  onClick={() => setHistoryChart("speed")}
                >
                  速度视图
                </button>
                <button
                  type="button"
                  aria-pressed={historyChart === "traffic"}
                  onClick={() => setHistoryChart("traffic")}
                >
                  流量视图
                </button>
              </div>
            </div>
            <TrafficChart
              mode="history"
              history={history.view?.chart}
              historyView={historyChart}
              historyLabelMode={
                selection.kind === "preset" &&
                (selection.preset === "today" ||
                  selection.preset === "yesterday")
                  ? "time"
                  : "date"
              }
            />
          </section>
          <div className="history-details">
            <section className="detail-panel">
              <div className="section-heading">
                <div>
                  <span className="eyebrow">SPEED / 02</span>
                  <h2>速率分布</h2>
                </div>
              </div>
              <div className="detail-metric-grid">
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
              </div>
            </section>
            <section className="detail-panel">
              <div className="section-heading">
                <div>
                  <span className="eyebrow">CONFIDENCE / 03</span>
                  <h2>数据质量</h2>
                </div>
              </div>
              <QualityContent
                liveMode={false}
                status={status}
                coverage={history.targets?.connectionCoverage ?? null}
                retention={history.targets?.dimensionRetention ?? null}
                otherBytes={history.targets?.otherBytes ?? null}
                history={history.view}
                observedAt={null}
                intervalMillis={null}
              />
            </section>
          </div>
          <section className="history-targets">
            <div className="section-heading">
              <div>
                <span className="eyebrow">CONTRIBUTORS / 04</span>
                <h2>维度贡献</h2>
              </div>
              <button
                className="text-link"
                type="button"
                onClick={() => navigate("targets")}
              >
                查看全部与搜索 <span aria-hidden="true">↗</span>
              </button>
            </div>
            <DimensionSelect value={by} onChange={setBy} />
            <TargetList
              historical={historicalRows}
              historicalGlobalBytes={history.targets?.globalBytes}
              available={history.targets?.available ?? true}
              noTraffic={history.targets?.noTraffic ?? false}
              loading={history.loading && history.targets === null}
              error={history.error && history.targets === null}
            />
          </section>
        </div>
      ) : null}

      {workspace === "health" ? (
        <div className="health-workspace workspace-enter">
          <section className="health-lead">
            <span className="eyebrow">OBSERVATION INTEGRITY</span>
            <h2>
              {status.status === "ok"
                ? "采集运行正常"
                : status.status === "degraded"
                  ? "采集处于降级状态"
                  : "采集不可用"}
            </h2>
            <p>
              全局字节数来自计数器；目标与维度来自连接采样，因此归因结果可能缺失或近似。
            </p>
            <div className="health-tags">
              <span>采集状态：{status.reason}</span>
              <span>时区：{status.timezone}</span>
              <span>版本：{status.version || "—"}</span>
            </div>
          </section>
          <div className="health-grid">
            <section className="detail-panel">
              <div className="section-heading">
                <div>
                  <span className="eyebrow">COVERAGE / 01</span>
                  <h2>数据质量</h2>
                </div>
              </div>
              {!liveMode && history.error ? (
                <p className="data-alert" role="alert">
                  历史质量查询失败，可能显示上次结果。
                </p>
              ) : null}
              <QualityContent
                liveMode={liveMode}
                status={status}
                coverage={
                  liveMode
                    ? live.connectionCoverage
                    : (history.targets?.connectionCoverage ?? null)
                }
                retention={
                  liveMode
                    ? null
                    : (history.targets?.dimensionRetention ?? null)
                }
                otherBytes={
                  liveMode ? null : (history.targets?.otherBytes ?? null)
                }
                history={history.view}
                observedAt={liveMode ? live.observedAt : null}
                intervalMillis={liveMode ? live.intervalMillis : null}
              />
            </section>
            <StoragePanel value={storage} error={storageError} />
          </div>
          <section className="detail-panel sessions-panel">
            <div className="section-heading">
              <div>
                <span className="eyebrow">RUNTIME / 03</span>
                <h2>最近运行</h2>
              </div>
              <span className="micro">采集进程上下文</span>
            </div>
            <RuntimeSessions sessions={sessions} error={sessionsError} />
          </section>
        </div>
      ) : null}
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

function Metric({
  label,
  value,
  accent,
}: {
  label: string;
  value: string;
  accent?: "download" | "upload";
}) {
  return (
    <div className={`metric ${accent ?? ""}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
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
function QualityContent({
  liveMode,
  status,
  coverage,
  retention,
  otherBytes,
  history,
  observedAt,
  intervalMillis,
}: {
  liveMode: boolean;
  status: StatusResponse;
  coverage: number | null;
  retention: number | null;
  otherBytes: ByteString | null;
  history: ReturnType<typeof useHistoryViewModel>["view"];
  observedAt: number | null;
  intervalMillis: number | null;
}) {
  return (
    <>
      <CoverageRow
        label="可归因覆盖"
        value={coverage}
        detail="全局流量中可关联到连接维度的比例。"
      />
      {!liveMode ? (
        <CoverageRow
          label="排行覆盖"
          value={retention}
          detail="可归因流量中保留在当前排行中的比例。"
        />
      ) : null}
      <dl className="quality-facts">
        <div>
          <dt>采集状态</dt>
          <dd>{status.reason}</dd>
        </div>
        {liveMode ? (
          <>
            <div>
              <dt>最后目标观测</dt>
              <dd>{formatClock(observedAt)}</dd>
            </div>
            <div>
              <dt>目标观测间隔</dt>
              <dd>{formatInterval(intervalMillis)}</dd>
            </div>
            <div>
              <dt>未归因累计</dt>
              <dd>实时不累计</dd>
            </div>
          </>
        ) : (
          <>
            <div>
              <dt>数据完整率</dt>
              <dd>{formatRatio(history?.completeness ?? null)}</dd>
            </div>
            <div>
              <dt>未归因流量</dt>
              <dd>{displayBytes(history?.unattributedBytes)}</dd>
            </div>
            <div>
              <dt>Top K 之外</dt>
              <dd>{displayBytes(otherBytes)}</dd>
            </div>
            <div>
              <dt>恢复流量</dt>
              <dd>{displayBytes(history?.recoveredBytes)}</dd>
            </div>
            <div>
              <dt>计数器重置</dt>
              <dd>{history?.resetCount ?? "—"}</dd>
            </div>
            <div>
              <dt>质量事件</dt>
              <dd>{history?.qualityEvents.length ?? "—"}</dd>
            </div>
            <div>
              <dt>边界估算</dt>
              <dd>
                {history?.boundaryApproximate === undefined
                  ? "—"
                  : history.boundaryApproximate
                    ? "已近似"
                    : "精确边界"}
              </dd>
            </div>
          </>
        )}
      </dl>
    </>
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
  const percent =
    value === null ? null : Math.max(0, Math.min(100, value * 100));
  return (
    <div className="coverage-row">
      <div className="coverage-label">
        <span className="label-with-tooltip">
          <strong>{label}</strong>
          <InfoTooltip content={detail} label={`查看“${label}”说明`} />
        </span>
        <strong>{formatRatio(value)}</strong>
      </div>
      <div
        className="coverage-track"
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent ?? undefined}
      >
        <i style={{ "--coverage": `${percent ?? 0}%` } as CSSProperties} />
      </div>
    </div>
  );
}
function RuntimeSessions({
  sessions,
  error,
}: {
  sessions: RuntimeSessionResponse[];
  error: boolean;
}) {
  if (error) return <p className="empty-state">运行记录暂时无法加载。</p>;
  if (sessions.length === 0)
    return <p className="empty-state">暂无运行记录。</p>;
  return (
    <div className="runtime-session-list">
      {sessions.map((session) => (
        <article
          className="runtime-session"
          key={`${session.started_at}-${session.last_seen_at}`}
        >
          <span className="session-marker" />
          <div>
            <strong>
              {session.ended_at === null ? "当前运行" : "历史运行"}
            </strong>
            <small>
              {session.sing_box_version.startsWith("sing-box")
                ? session.sing_box_version
                : `sing-box ${session.sing_box_version}`}
            </small>
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
  );
}
function formatClock(value: number | null): string {
  return value === null
    ? "—"
    : new Date(value * 1000).toLocaleTimeString("zh-CN", {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
}
function formatInterval(value: number | null): string {
  return value === null ? "—" : `${(value / 1000).toFixed(1)} 秒`;
}
function formatCount(value: number | null): string {
  return value === null ? "—" : value.toFixed(value % 1 === 0 ? 0 : 1);
}
function displayBytes(value: ByteString | null | undefined): string {
  return value === null || value === undefined ? "—" : formatBytes(value);
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
