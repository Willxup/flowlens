import type { ReactNode } from "react";
import type { ServiceLevel } from "../api/contracts";
import { AppFooter } from "../components/AppFooter";
import { ThemeSelect } from "../features/theme/ThemeSelect";

export type Workspace = "overview" | "targets" | "history" | "health";

const navigation: Array<{
  id: Workspace;
  title: string;
  caption: string;
  icon: ReactNode;
}> = [
  {
    id: "overview",
    title: "实时总览",
    caption: "Live signal",
    icon: <path d="M3 12h4l3-7 4 14 3-7h4" />,
  },
  {
    id: "targets",
    title: "目标探索",
    caption: "Destinations",
    icon: (
      <>
        <circle cx="12" cy="12" r="8" />
        <circle cx="12" cy="12" r="3" />
        <path d="M12 2v3m0 14v3M2 12h3m14 0h3" />
      </>
    ),
  },
  {
    id: "history",
    title: "历史分析",
    caption: "Timeline",
    icon: (
      <>
        <path d="M3 5v5h5M4 10a8 8 0 1 1 1.5 7" />
        <path d="M12 7v5l3 2" />
      </>
    ),
  },
  {
    id: "health",
    title: "质量与存储",
    caption: "Integrity",
    icon: (
      <>
        <path d="M12 2 4 5v6c0 5.5 3.2 8.7 8 11 4.8-2.3 8-5.5 8-11V5l-8-3Z" />
        <path d="m9 12 2 2 4-4" />
      </>
    ),
  },
];

export function Shell({
  status,
  version,
  sourceMode,
  authEnabled,
  onLogout,
  logoutFailed,
  workspace,
  onNavigate,
  children,
}: {
  status: ServiceLevel;
  version: string;
  sourceMode: "app" | "demo";
  authEnabled: boolean;
  onLogout: () => void;
  logoutFailed?: boolean;
  workspace: Workspace;
  onNavigate: (workspace: Workspace) => void;
  children: ReactNode;
}) {
  const current = navigation.find((item) => item.id === workspace)!;
  const logoutLabel = logoutFailed ? "退出失败，请重试" : "退出";
  return (
    <div className="app-shell" data-source-mode={sourceMode}>
      <a className="skip-link" href="#workspace-content">
        跳转到内容
      </a>
      <aside className="sidebar" aria-label="工作台侧栏">
        <a
          className="brand"
          href="https://github.com/Willxup/flowlens"
          target="_blank"
          rel="noreferrer"
          aria-label="FlowLens GitHub 仓库"
        >
          <span className="brand-mark" aria-hidden="true">
            <span />
          </span>
          <span className="brand-type">
            <strong>FlowLens</strong>
            <small>TRAFFIC OBSERVATORY</small>
          </span>
        </a>
        <div className="sidebar-section-label">
          WORKSPACES <span>01 — 04</span>
        </div>
        <nav className="side-nav" aria-label="工作区">
          {navigation.map((item, index) => (
            <button
              key={item.id}
              type="button"
              className="nav-item"
              aria-current={workspace === item.id ? "page" : undefined}
              onClick={() => onNavigate(item.id)}
            >
              <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.7"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                {item.icon}
              </svg>
              <span className="nav-copy">
                <strong>{item.title}</strong>
                <small>{item.caption}</small>
              </span>
              <span className="nav-index">0{index + 1}</span>
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="sidebar-note">
            <span className="note-glyph" aria-hidden="true">
              ↗
            </span>
            <div>
              <strong>全局流量精确记录</strong>
              <span>目标归因来自连接采样</span>
            </div>
          </div>
          <AppFooter version={version} />
        </div>
      </aside>
      <div className="workspace-shell">
        <header className="topbar">
          <div className="topbar-context">
            <span>工作台</span>
            <span className="context-divider">/</span>
            <strong>{current.title}</strong>
          </div>
          <div className="top-actions">
            <span className={`live-status ${status}`} role="status">
              <i />
              {status === "ok"
                ? "采集正常"
                : status === "degraded"
                  ? "采集降级"
                  : "采集失败"}
            </span>
            {sourceMode === "demo" ? (
              <span className="demo-chip">离线演示</span>
            ) : null}
            <ThemeSelect />
            {authEnabled ? (
              <button
                className={`logout-button${logoutFailed ? " failed" : ""}`}
                type="button"
                aria-label={logoutLabel}
                onClick={onLogout}
              >
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M10 4H5.5A1.5 1.5 0 0 0 4 5.5v13A1.5 1.5 0 0 0 5.5 20H10M14.5 8.5 18 12l-3.5 3.5M9 12h9" />
                </svg>
                <span>{logoutFailed ? "重试退出" : "退出"}</span>
              </button>
            ) : null}
          </div>
        </header>
        <main className="app" id="workspace-content">
          {children}
        </main>
        <div className="mobile-footer">
          <AppFooter version={version} />
        </div>
        <nav className="mobile-nav" aria-label="移动工作区">
          {navigation.map((item) => (
            <button
              key={item.id}
              type="button"
              aria-current={workspace === item.id ? "page" : undefined}
              onClick={() => onNavigate(item.id)}
            >
              <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                {item.icon}
              </svg>
              <span>{item.title}</span>
            </button>
          ))}
        </nav>
      </div>
    </div>
  );
}
