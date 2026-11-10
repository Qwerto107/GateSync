import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Archive, Check } from "lucide-react";
import { api } from "../api";
import { Spin } from "./ui";

export default function LogRetentionSettings() {
  const [saved, setSaved] = useState<number>();
  const [days, setDays] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setError("");
    void api<{ retentionDays: number }>("/log-settings")
      .then((value) => {
        if (!cancelled) {
          setSaved(value.retentionDays);
          setDays(String(value.retentionDays));
        }
      })
      .catch((e) => {
        if (!cancelled) setError(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [retry]);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const value = Number(days);
    if (!/^\d+$/.test(days) || !Number.isInteger(value) || value > 36500) {
      setError("请输入 0–36500 的整数，0 表示永久保留。");
      return;
    }
    setBusy(true);
    setSuccess(false);
    setError("");
    try {
      const result = await api<{ retentionDays: number }>(
        "/log-settings",
        "PUT",
        { retentionDays: value },
      );
      setSaved(result.retentionDays);
      setDays(String(result.retentionDays));
      setSuccess(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setBusy(false);
    }
  };
  const shorter =
    saved !== undefined &&
    Number(days) > 0 &&
    (saved === 0 || Number(days) < saved);
  return (
    <form
      className="card settings-card system-card log-retention-card"
      onSubmit={save}
    >
      <div className="settings-header">
        <span className="icon-soft">
          <Archive size={20} />
        </span>
        <div>
          <h2>日志保留</h2>
          <p>后台每日自动清理，不受自动同步开关影响</p>
        </div>
      </div>
      <div className="settings-section">
        {saved === undefined ? (
          error ? (
            <div className="notice danger" role="alert">
              {error}
              <button
                type="button"
                className="text-btn"
                onClick={() => setRetry((v) => v + 1)}
              >
                重试
              </button>
            </div>
          ) : (
            <span className="inline muted">
              <Spin />
              正在读取日志保留设置…
            </span>
          )
        ) : (
          <>
            <label className="field retention-days">
              日志保留天数
              <input
                type="text"
                inputMode="numeric"
                pattern="[0-9]+"
                maxLength={5}
                required
                disabled={busy}
                value={days}
                onChange={(e) => {
                  setDays(e.target.value);
                  setSuccess(false);
                }}
                aria-label="日志保留天数"
              />
              <span className="field-hint">
                0 表示永久保留。设置按每 24
                小时清理任务生效，保存时不会立即删除。
              </span>
            </label>
            {shorter && (
              <div className="notice warning">
                保留时间将缩短。下次清理会永久删除超期日志，请按需先导出备份。
              </div>
            )}
            {error && (
              <div className="notice danger" role="alert">
                {error}
              </div>
            )}
            {success && (
              <div role="status" className="notice info">
                日志保留设置已保存。
              </div>
            )}
          </>
        )}
      </div>
      <div className="settings-bottom">
        <span>独立保存，不影响当前登录会话</span>
        <button className="btn primary" disabled={saved === undefined || busy}>
          {busy ? <Spin /> : <Check size={16} />}保存日志设置
        </button>
      </div>
    </form>
  );
}
