import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  Check,
  Globe2,
  LockKeyhole,
  ShieldCheck,
  TriangleAlert,
} from "lucide-react";
import { api } from "../api";
import { Spin, Toggle } from "../components/ui";
import LogRetentionSettings from "../components/LogRetentionSettings";

interface SystemConfig {
  publicUrl: string;
  googleEnabled: boolean;
  googleClientId: string;
  googleAllowedEmails: string[];
  hasGoogleSecret: boolean;
}

export default function System({ onSaved }: { onSaved: () => Promise<void> }) {
  const [config, setConfig] = useState<SystemConfig>();
  const [secret, setSecret] = useState("");
  const [emails, setEmails] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let cancelled = false;
    void api<SystemConfig>("/system")
      .then((value) => {
        if (!cancelled) {
          setConfig(value);
          setEmails(value.googleAllowedEmails.join("\n"));
          setError("");
        }
      })
      .catch((e) => {
        if (!cancelled) setError(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [retry]);
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!config) return;
    if (newPassword !== confirmPassword) {
      setError("两次输入的新密码不一致");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api("/system", "PUT", {
        publicUrl: config.publicUrl,
        googleEnabled: config.googleEnabled,
        googleClientId: config.googleClientId,
        googleClientSecret: secret,
        googleAllowedEmails: emails
          .split(/[,\n]/)
          .map((v) => v.trim())
          .filter(Boolean),
        currentPassword,
        newPassword,
      });
      setSecret("");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      await onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <div className="page-title">
        <div>
          <div className="eyebrow muted">站点与登录</div>
          <h1>系统设置</h1>
          <p>管理访问地址、Google 登录和访问密码，保存后生效。</p>
        </div>
      </div>
      {!config ? (
        <div className="card table-loading">
          {error ? (
            <>
              <span className="error-text">{error}</span>
              <button className="btn" onClick={() => setRetry((v) => v + 1)}>
                重试
              </button>
            </>
          ) : (
            <>
              <Spin />
              正在读取设置…
            </>
          )}
        </div>
      ) : (
        <form className="card settings-card system-card" onSubmit={save}>
          <fieldset disabled={busy} className="system-fieldset">
            <div className="settings-header">
              <span className="icon-soft">
                <Globe2 size={20} />
              </span>
              <div>
                <h2>站点地址</h2>
                <p>用于生成 OAuth 回调与会话安全属性</p>
              </div>
            </div>
            <div className="settings-section">
              <label className="field">
                公开访问地址
                <input
                  type="url"
                  required
                  value={config.publicUrl}
                  onChange={(e) =>
                    setConfig({ ...config, publicUrl: e.target.value })
                  }
                  placeholder="http://localhost:13850"
                />
                <span className="field-hint">
                  填写完整 HTTP(S) 地址，不含路径。修改后需同步更新 Google
                  控制台中的回调 URI。
                </span>
              </label>
            </div>
            <div className="settings-header">
              <span className="icon-soft">
                <ShieldCheck size={20} />
              </span>
              <div>
                <h2>Google OAuth</h2>
                <p>仅允许白名单中的已验证邮箱登录</p>
              </div>
              <Toggle
                checked={config.googleEnabled}
                onChange={(v) => setConfig({ ...config, googleEnabled: v })}
                label="启用 Google 登录"
                disabled={busy}
              />
            </div>
            <div className="settings-section system-fields">
              <label className="field">
                Client ID
                <input
                  value={config.googleClientId}
                  onChange={(e) =>
                    setConfig({ ...config, googleClientId: e.target.value })
                  }
                  required={config.googleEnabled}
                  autoComplete="off"
                  maxLength={1024}
                />
              </label>
              <label className="field">
                Client Secret
                <input
                  type="password"
                  value={secret}
                  onChange={(e) => setSecret(e.target.value)}
                  placeholder={
                    config.hasGoogleSecret
                      ? "已保存，留空表示不修改"
                      : "请输入 Google Client Secret"
                  }
                  required={config.googleEnabled && !config.hasGoogleSecret}
                  autoComplete="new-password"
                  maxLength={4096}
                />
                <span className="field-hint">
                  <LockKeyhole size={12} />
                  加密保存，保存后不再回显
                </span>
              </label>
              <label className="field">
                邮箱白名单
                <textarea
                  rows={4}
                  value={emails}
                  onChange={(e) => setEmails(e.target.value)}
                  placeholder="每行一个完整邮箱，或用逗号分隔"
                  required={config.googleEnabled}
                />
              </label>
              <div className="notice info">
                <Globe2 size={16} />
                <span>
                  授权回调 URI：
                  <span className="mono break-word">
                    {config.publicUrl.replace(/\/$/, "")}
                    /api/auth/google/callback
                  </span>
                </span>
              </div>
            </div>
            <div className="settings-header">
              <span className="icon-soft">
                <LockKeyhole size={20} />
              </span>
              <div>
                <h2>访问密码</h2>
                <p>留空保留当前密码；修改时需验证当前密码</p>
              </div>
            </div>
            <div className="settings-section system-fields">
              <label className="field">
                当前密码
                <input
                  type="password"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  autoComplete="current-password"
                  required={!!newPassword}
                  maxLength={72}
                />
              </label>
              <div className="form-grid">
                <label className="field">
                  新密码
                  <input
                    type="password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    autoComplete="new-password"
                    maxLength={72}
                    placeholder="12–72 字节"
                  />
                </label>
                <label className="field">
                  确认新密码
                  <input
                    type="password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    autoComplete="new-password"
                    required={!!newPassword}
                    maxLength={72}
                  />
                </label>
              </div>
            </div>
          </fieldset>
          {error && (
            <div className="notice danger system-error" role="alert">
              <TriangleAlert size={16} />
              {error}
            </div>
          )}
          <div className="settings-bottom">
            <span>
              <LockKeyhole size={13} />
              保存后将撤销现有会话，需重新登录
            </span>
            <button className="btn primary" disabled={busy}>
              {busy ? <Spin /> : <Check size={16} />}保存设置
            </button>
          </div>
        </form>
      )}
      <LogRetentionSettings />
    </>
  );
}
