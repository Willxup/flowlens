import type { HistoricalTargetRow } from "./TargetList";

export function Topology({ targets }: { targets: HistoricalTargetRow[] }) {
  const nodes = targets.slice(0, 3);
  const lastMobileTargetY = 162.5 + (nodes.length - 1) * 50;
  return (
    <div
      className="topology"
      aria-label="流量拓扑：来源网络经过代理网关到目标服务"
    >
      <svg
        className="topology-desktop-flow"
        viewBox="0 0 900 300"
        aria-hidden="true"
      >
        <path className="flow-path" d="M130 72 C300 72 300 150 450 150" />
        <path
          className="flow-path secondary"
          d="M130 232 C300 232 300 150 450 150"
        />
        {nodes.map((_, index) => (
          <path
            key={index}
            className="flow-path target"
            d={`M450 150 C610 150 610 ${50 + index * 100} 770 ${50 + index * 100}`}
          />
        ))}
      </svg>
      <svg
        className="topology-mobile-flow"
        viewBox="0 0 300 285"
        preserveAspectRatio="none"
        aria-hidden="true"
      >
        <defs>
          <marker
            id="topology-mobile-arrow"
            viewBox="0 0 6 6"
            refX="5.5"
            refY="3"
            markerWidth="5"
            markerHeight="5"
            orient="auto"
          >
            <path className="mobile-flow-arrow" d="M0 0L6 3L0 6Z" />
          </marker>
        </defs>
        <path className="flow-path" d="M75 50V60H138V72" />
        <path className="flow-path secondary" d="M225 50V60H162V72" />
        {nodes.length > 0 ? (
          <path
            className="flow-path target mobile-target-trunk"
            d={`M150 120V128H14V${lastMobileTargetY}`}
          />
        ) : null}
        {nodes.map((_, index) => (
          <path
            key={index}
            className="flow-path target mobile-target-path"
            d={`M14 ${162.5 + index * 50}H24`}
            markerEnd="url(#topology-mobile-arrow)"
          />
        ))}
      </svg>
      <div className="flow-node node-source-one">
        <strong>来源网络</strong>
        <span>采样可见来源</span>
      </div>
      <div className="flow-node node-source-two">
        <strong>其他来源</strong>
        <span>Top 20 合并展示</span>
      </div>
      <div className="flow-node node-gateway">
        <strong>代理网关</strong>
        <span>network edge</span>
      </div>
      {nodes.map((node, index) => (
        <div
          key={node.rawValue}
          className={`flow-node node-target node-target-${index}`}
        >
          <strong>{node.displayName}</strong>
          <span>{node.rawValue}</span>
        </div>
      ))}
    </div>
  );
}
