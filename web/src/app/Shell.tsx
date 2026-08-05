import type { ReactNode } from "react";
import type { ServiceLevel } from "../api/contracts";
import { AppFooter } from "../components/AppFooter";
import { Tooltip } from "../components/Tooltip";
import { ThemeSelect } from "../features/theme/ThemeSelect";

export function Shell({
  status,
  version,
  sourceMode,
  authEnabled,
  onLogout,
  logoutFailed,
  children,
}: {
  status: ServiceLevel;
  version: string;
  sourceMode: "app" | "demo";
  authEnabled: boolean;
  onLogout: () => void;
  logoutFailed?: boolean;
  children: ReactNode;
}) {
  const logoutLabel = logoutFailed ? "退出失败，请重试" : "退出";
  return (
    <div className="app-shell" data-source-mode={sourceMode}>
      <header className="topbar">
        <a
          className="brand"
          href="https://github.com/Willxup/flowlens"
          target="_blank"
          rel="noreferrer"
          aria-label="FlowLens GitHub 仓库"
        >
          <div className="brand-mark" aria-hidden="true" />
          <div>
            <strong>FlowLens</strong>
            <span className="eyebrow">network telemetry</span>
          </div>
        </a>
        <div className="top-actions">
          <span className={`live-status ${status}`}>
            <i />
            {status === "ok"
              ? "采集正常"
              : status === "degraded"
                ? "采集降级"
                : "采集失败"}
          </span>
          <ThemeSelect />
          {authEnabled ? (
            <Tooltip content={logoutLabel}>
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
            </Tooltip>
          ) : null}
        </div>
      </header>
      <main className="app">{children}</main>
      <AppFooter version={version} />
    </div>
  );
}
