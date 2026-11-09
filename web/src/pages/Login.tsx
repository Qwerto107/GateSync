import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  ArrowRight,
  ArrowUpRight,
  Cloud,
  Eye,
  EyeOff,
  Globe2,
  LockKeyhole,
  ShieldCheck,
  TriangleAlert,
} from "lucide-react";
import { api } from "../api";
import { Brand, Spin } from "../components/ui";

export default function Login({ onLogin }: { onLogin: () => Promise<void> }) {
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(true);
  const [show, setShow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [google, setGoogle] = useState(false);
  const [configError, setConfigError] = useState(false);
  const initialError = new URLSearchParams(location.search).get("login_error");
  const [error, setError] = useState(
    initialError
      ? "Google 登录未完成，请检查账号白名单与 OAuth 配置，或使用访问密码。"
      : "",
  );
  useEffect(() => {
    void api<{ google: boolean }>("/auth/config")
      .then((v) => setGoogle(v.google))
      .catch(() => setConfigError(true));
    if (initialError) history.replaceState(null, "", location.pathname);
  }, []);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/auth/login", "POST", { password, remember });
      setPassword("");
      await onLogin();
    } catch (e) {
      setError(e instanceof Error ? e.message : "登录失败");
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="login-page">
      <section className="login-story">
        <Brand />
        <div className="story-content">
          <span className="eyebrow">
            <span className="status-dot" />
            让安全连接，始终在线
          </span>
          <h1>
            IP 变了，
            <br />
            安全组<span>自动跟上。</span>
          </h1>
          <p>
            在一处管理云安全组的放行规则。
            <br />
            定时检测公网 IP，变更后自动同步。
          </p>
          <div className="connection-visual">
            <div className="connection-node">
              <Globe2 size={25} />
              <span>当前公网 IP</span>
            </div>
            <div className="connection-line">
              <span />
              <ArrowRight size={19} />
              <small>安全同步</small>
            </div>
            <div className="connection-node cloud-node">
              <Cloud size={26} />
              <span>腾讯云安全组</span>
            </div>
          </div>
          <div className="story-note">
            <ShieldCheck size={16} />
            <span>仅更新授权规则，保留其他安全组配置</span>
          </div>
        </div>
        <div className="story-footer">
          为每一次远程连接，少一次手动配置。<span>GATESYNC / 01</span>
        </div>
      </section>
      <section className="login-form-section">
        <div className="login-card">
          <span className="login-lock">
            <LockKeyhole size={24} />
          </span>
          <h2>登录 GateSync</h2>
          <p className="muted">欢迎回来，继续管理你的安全连接</p>
          <button
            className="btn google-btn"
            disabled={!google || busy}
            onClick={() => {
              location.href = "/api/auth/google";
            }}
          >
            <span className="google-g">G</span>使用 Google 登录
            <ArrowUpRight size={16} />
          </button>
          {!google && (
            <p className="field-hint center">
              {configError
                ? "暂时无法读取 Google 登录配置"
                : "在系统设置中配置 Google OAuth 后即可使用"}
            </p>
          )}
          <div className="or-divider">
            <span>或使用访问密码</span>
          </div>
          <form onSubmit={submit}>
            <label className="field">
              访问密码
              <div className="password-field">
                <input
                  type={show ? "text" : "password"}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="请输入访问密码"
                  autoComplete="current-password"
                  required
                  maxLength={72}
                />
                <button
                  type="button"
                  className="icon-btn"
                  aria-label={show ? "隐藏密码" : "显示密码"}
                  onClick={() => setShow(!show)}
                >
                  {show ? <EyeOff size={17} /> : <Eye size={17} />}
                </button>
              </div>
            </label>
            <label className="check-label remember">
              <input
                type="checkbox"
                checked={remember}
                onChange={(e) => setRemember(e.target.checked)}
              />
              在此设备上保持登录 7 天
            </label>
            {error && (
              <div className="notice danger" role="alert">
                <TriangleAlert size={16} />
                {error}
              </div>
            )}
            <button className="btn primary login-submit" disabled={busy}>
              {busy ? <Spin /> : null}登录
              <ArrowRight size={17} />
            </button>
          </form>
          <div className="login-restriction">
            <LockKeyhole size={13} />
            仅限已授权用户登录
          </div>
        </div>
        <p className="login-bottom">GateSync · 你的云端连接助手</p>
      </section>
    </div>
  );
}
