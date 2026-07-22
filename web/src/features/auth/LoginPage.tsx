import { type FormEvent, useState } from "react";
import type { FlowLensDataSource } from "../../api/source";
import { ThemeSelect } from "../theme/ThemeSelect";

interface LoginPageProps {
  source: FlowLensDataSource;
  onAuthenticated: () => void;
}

export function LoginPage({ source, onAuthenticated }: LoginPageProps) {
  const [accessKey, setAccessKey] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [failed, setFailed] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (accessKey.length === 0 || submitting) return;
    const value = accessKey;
    setSubmitting(true);
    setFailed(false);
    try {
      await source.login(value);
      setAccessKey("");
      onAuthenticated();
    } catch {
      setAccessKey("");
      setFailed(true);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="login-page">
      <div className="login-theme">
        <ThemeSelect />
      </div>
      <section className="login-card" aria-labelledby="login-title">
        <div className="brand-mark" aria-hidden="true" />
        <span className="eyebrow">Private network observatory</span>
        <h1 id="login-title">进入 FlowLens</h1>
        <p>使用配置文件中的共享访问密钥继续。密钥只用于本次登录请求。</p>
        <form onSubmit={submit}>
          <label className="field">
            <span>共享访问密钥</span>
            <input
              autoComplete="current-password"
              autoFocus
              type="password"
              value={accessKey}
              onChange={(event) => setAccessKey(event.target.value)}
            />
          </label>
          {failed ? (
            <p role="alert" className="form-error">
              无法登录，请检查密钥后重试。
            </p>
          ) : null}
          <button
            className="primary-button"
            disabled={accessKey.length === 0 || submitting}
            type="submit"
          >
            {submitting ? "正在验证…" : "进入 FlowLens"}
          </button>
        </form>
      </section>
    </main>
  );
}
