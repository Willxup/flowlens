import type { ReactNode } from "react";
import type { ServiceLevel } from "../api/contracts";
import { ThemeSelect } from "../features/theme/ThemeSelect";

export function Shell({
  status,
  sourceMode,
  authEnabled,
  onLogout,
  logoutFailed,
  children,
}: {
  status: ServiceLevel;
  sourceMode: "app" | "demo";
  authEnabled: boolean;
  onLogout: () => void;
  logoutFailed?: boolean;
  children: ReactNode;
}) {
  const logoutLabel = logoutFailed ? "退出失败，请重试" : "退出";
  return (
    <main className="app" data-source-mode={sourceMode}>
      <header className="topbar">
        <div className="brand">
          <div className="brand-mark" aria-hidden="true" />
          <div>
            <strong>FlowLens</strong>
            <span className="eyebrow">edge telemetry</span>
          </div>
        </div>
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
            <button
              className={`logout-button${logoutFailed ? " failed" : ""}`}
              type="button"
              aria-label={logoutLabel}
              title={logoutLabel}
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
      {children}
    </main>
  );
}
