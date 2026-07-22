import type { HistoricalTargetRow } from "./TargetList";

export function Topology({ targets }: { targets: HistoricalTargetRow[] }) {
  const nodes = targets.slice(0, 3);
  return (
    <div
      className="topology"
      aria-label="流量拓扑：来源网络经过代理网关到目标服务"
    >
      <svg viewBox="0 0 900 300" aria-hidden="true">
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
