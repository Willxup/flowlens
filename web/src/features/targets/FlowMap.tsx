import type { LiveTargetResponse } from "../../api/contracts";
import { formatRate, formatRatio } from "../../lib/format";

const rowCenters = [50, 107, 164, 221, 278, 335];

export function FlowMap({
  targets,
  globalRate,
  coverage,
  state,
}: {
  targets: LiveTargetResponse[];
  globalRate: number | null;
  coverage: number | null;
  state: "loading" | "fresh" | "stale" | "error";
}) {
  const ranked = [...targets]
    .sort(
      (a, b) =>
        b.upload_bytes_per_second +
        b.download_bytes_per_second -
        (a.upload_bytes_per_second + a.download_bytes_per_second),
    )
    .slice(0, 5);
  const shownRate = ranked.reduce(
    (sum, item) =>
      sum + item.upload_bytes_per_second + item.download_bytes_per_second,
    0,
  );
  const residual =
    globalRate === null || globalRate <= 0 || shownRate > globalRate
      ? null
      : globalRate - shownRate;
  const branches = ranked.map((item, index) => {
    const rate = item.upload_bytes_per_second + item.download_bytes_per_second;
    return { rate, y: rowCenters[index]!, key: item.raw_endpoint, rest: false };
  });
  if (residual !== null)
    branches.push({
      rate: residual,
      y: rowCenters[ranked.length]!,
      key: "residual",
      rest: true,
    });
  return (
    <section className="flow-map" aria-labelledby="flow-map-title">
      <div className="section-heading">
        <div>
          <span className="eyebrow">FLOW ATTRIBUTION / 01</span>
          <h2 id="flow-map-title">流向构成</h2>
        </div>
        <span className="micro">同次目标快照 · 速率权重</span>
      </div>
      <div className="flow-canvas" data-flow-state={state}>
        {ranked.length > 0 ? (
          <svg
            className="flow-network"
            viewBox="0 0 1000 380"
            preserveAspectRatio="none"
            aria-hidden="true"
          >
            <defs>
              <linearGradient
                id="flow-branch-gradient"
                x1="0"
                y1="0"
                x2="1"
                y2="0"
              >
                <stop
                  offset="0"
                  stopColor="var(--download)"
                  stopOpacity=".15"
                />
                <stop
                  offset=".45"
                  stopColor="var(--download)"
                  stopOpacity=".65"
                />
                <stop offset="1" stopColor="var(--download)" stopOpacity=".3" />
              </linearGradient>
            </defs>
            <path
              className="flow-trunk"
              d="M260 188 C330 188 350 188 415 188"
              strokeWidth={globalRate === null || globalRate <= 0 ? 2 : 24}
            />
            {branches.map((branch) => {
              const share =
                globalRate !== null && globalRate > 0
                  ? Math.max(0, Math.min(1, branch.rate / globalRate))
                  : 0;
              const width =
                branch.rate <= 0
                  ? 1
                  : Math.max(2, Math.min(24, 24 * Math.sqrt(share)));
              const path = `M415 188 C510 188 525 ${branch.y} 625 ${branch.y}`;
              return (
                <g
                  key={branch.key}
                  className={
                    branch.rest ? "flow-rest-branch" : "flow-target-branch"
                  }
                >
                  <path className="flow-ribbon" d={path} strokeWidth={width} />
                  {state === "fresh" && branch.rate > 0 ? (
                    <path className="flow-particles" d={path} strokeWidth="2" />
                  ) : null}
                </g>
              );
            })}
            <circle cx="415" cy="188" r="4" className="flow-junction" />
          </svg>
        ) : null}
        <div className="flow-origin">
          <span className="origin-icon" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <span>全局速率</span>
          <strong>{formatRate(globalRate)}</strong>
          <small>COUNTER / PRECISE</small>
        </div>
        <div className="flow-destinations">
          {state === "loading" && ranked.length === 0 ? (
            <p className="empty-state">目标快照正在加载。</p>
          ) : null}
          {state === "error" && ranked.length === 0 ? (
            <p className="empty-state">无法读取目标快照。</p>
          ) : null}
          {ranked.length === 0 && state !== "loading" && state !== "error" ? (
            <p className="empty-state">
              {globalRate === 0
                ? "当前全局速率为 0。"
                : "当前没有可展示的目标归因。"}
            </p>
          ) : null}
          {ranked.map((item, index) => {
            const rate =
              item.upload_bytes_per_second + item.download_bytes_per_second;
            const share =
              globalRate !== null && globalRate > 0 ? rate / globalRate : null;
            return (
              <div className="flow-destination" key={item.raw_endpoint}>
                <span className="flow-rank">0{index + 1}</span>
                <div className="flow-target-name">
                  <strong title={item.display_name}>
                    {targetName(item.raw_endpoint, item.display_name)}
                  </strong>
                  <small title={item.raw_endpoint}>{item.raw_endpoint}</small>
                </div>
                <div className="flow-target-rate">
                  <strong>{formatRate(rate)}</strong>
                  <small>
                    {globalRate === 0
                      ? rate > 0
                        ? "估算超全局"
                        : "全局速率为 0"
                      : share === null
                        ? "占比未知"
                        : share > 1
                          ? "估算超全局"
                          : `约 ${formatRatio(share)}`}
                  </small>
                </div>
                <span
                  className="flow-mobile-weight"
                  style={{
                    width: `${share === null ? 0 : Math.max(0, Math.min(100, share * 100))}%`,
                  }}
                  aria-hidden="true"
                />
              </div>
            );
          })}
          {residual !== null ? (
            <div className="flow-destination flow-remainder">
              <span className="flow-rank">··</span>
              <div className="flow-target-name">
                <strong>未在前五展示</strong>
                <small>全局与上述近似速率的差额</small>
              </div>
              <div className="flow-target-rate">
                <strong>约 {formatRate(residual)}</strong>
                <small>估算余量</small>
              </div>
            </div>
          ) : shownRate > (globalRate ?? 0) && ranked.length > 0 ? (
            <div className="flow-destination flow-remainder">
              <span className="flow-rank">··</span>
              <div className="flow-target-name">
                <strong>无法拆分余量</strong>
                <small>目标估算之和超过全局速率</small>
              </div>
            </div>
          ) : null}
        </div>
      </div>
      <div className="flow-footnote">
        <span>
          {state === "fresh"
            ? "目标快照正常"
            : state === "loading"
              ? "正在载入目标快照"
              : state === "stale"
                ? "目标快照已过期"
                : "目标快照读取失败"}
        </span>
        <span>可归因覆盖 {formatRatio(coverage)}</span>
        <span>目标速率为近似归因；差额不代表精确的未归因流量</span>
      </div>
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
