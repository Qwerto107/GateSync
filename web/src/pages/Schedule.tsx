import { useEffect, useState } from "react";
import {
  Check,
  CheckCircle2,
  Clock3,
  Globe2,
  LockKeyhole,
  Plus,
  RefreshCw,
  X,
} from "lucide-react";
import { api, date, Log, Settings, State } from "../api";
import { Spin, Empty, Toggle, Select } from "../components/ui";

export default function Schedule({
  state,
  busy,
  onSave,
}: {
  state: State;
  busy: boolean;
  onSave: (s: Settings) => void;
}) {
  const [form, setForm] = useState(state.settings);
  const [useCron, setUseCron] = useState(!!state.settings.cron);
  const [records, setRecords] = useState<Log[]>([]);
  useEffect(() => {
    setForm(state.settings);
    setUseCron(!!state.settings.cron);
  }, [JSON.stringify(state.settings)]);
  useEffect(() => {
    void api<{ items: Log[] }>("/logs?days=7")
      .then((v) =>
        setRecords(
          v.items
            .filter((l) => l.kind === "ip" || l.kind === "sync")
            .slice(0, 6),
        ),
      )
      .catch(() => {});
  }, [state.ip.checkedAt, state.settings.enabled]);
  return (
    <>
      <div className="page-title">
        <div>
          <div className="eyebrow muted">后台自动化</div>
          <h1>计划任务</h1>
          <p>定时检查公网 IP，让授权规则持续保持一致。</p>
        </div>
        <span
          className={`badge ${state.settings.enabled ? "good" : "neutral"}`}
        >
          <span className="status-dot" />
          {state.settings.enabled ? "自动同步运行中" : "自动同步已暂停"}
        </span>
      </div>
      <div className="schedule-grid">
        <form
          className="card settings-card"
          onSubmit={(e) => {
            e.preventDefault();
            onSave({
              ...form,
              cron: useCron ? form.cron : "",
              sources: form.sources.filter((s) => s.trim()),
            });
          }}
        >
          <div className="settings-header">
            <span className="icon-soft">
              <RefreshCw size={20} />
            </span>
            <div>
              <h2>自动同步</h2>
              <p>应用运行期间，按计划检查并同步</p>
            </div>
            <Toggle
              checked={form.enabled}
              onChange={(v) => setForm({ ...form, enabled: v })}
              label="开启自动同步"
            />
          </div>
          <div className="settings-section">
            <label className="field">
              任务时区
              <Select
                label="任务时区"
                value={form.timezone}
                required
                searchable
                allowCustom
                onChange={(value) => setForm({ ...form, timezone: value })}
                placeholder="Asia/Shanghai"
                options={[
                  "Asia/Shanghai",
                  "Asia/Singapore",
                  "Asia/Tokyo",
                  "UTC",
                ].map((value) => ({ value, label: value }))}
              />
              <span className="field-hint">
                使用 IANA 时区名称，保存后按此时区解析 Cron。
              </span>
            </label>
          </div>
          <div className="settings-section">
            <label className="field-label">检查频率</label>
            <div className="segmented">
              {[1, 5, 15].map((n) => (
                <button
                  type="button"
                  key={n}
                  className={!useCron && form.interval === n ? "selected" : ""}
                  onClick={() => {
                    setUseCron(false);
                    setForm({ ...form, interval: n });
                  }}
                >
                  {n} 分钟
                </button>
              ))}
              <button
                type="button"
                className={useCron ? "selected" : ""}
                onClick={() => setUseCron(true)}
              >
                Cron
              </button>
            </div>
            {useCron && (
              <label className="field cron-field">
                <input
                  className="mono"
                  required
                  value={form.cron}
                  placeholder="*/5 * * * *"
                  onChange={(e) => setForm({ ...form, cron: e.target.value })}
                />
                <span className="field-hint">
                  五段表达式，使用上方配置的任务时区。
                </span>
              </label>
            )}
          </div>
          <div className="settings-section ip-query-settings">
            <span className="field-label">公网 IP 查询</span>
            <label className="check-label">
              <input
                type="checkbox"
                checked={form.detectOnManualSync}
                disabled={busy}
                onChange={(e) =>
                  setForm({ ...form, detectOnManualSync: e.target.checked })
                }
              />
              手动同步时查询公网 IP
            </label>
            <label className="check-label">
              <input
                type="checkbox"
                checked={form.detectOnSchedule}
                disabled={busy}
                onChange={(e) =>
                  setForm({ ...form, detectOnSchedule: e.target.checked })
                }
              />
              定时任务时查询公网 IP
            </label>
            <p className="field-hint">
              未勾选时使用上次成功查询的缓存。保存或编辑目标不主动查询，“重新检测”始终可用。定时查询随上方自动同步计划执行。
            </p>
          </div>
          <div className="settings-section">
            <label className="field-label">
              IP 检测源<span className="label-note">按顺序尝试</span>
            </label>
            {form.sources.map((source, i) => (
              <div className="source-row" key={i}>
                <span className="source-index">{i + 1}</span>
                <input
                  type="url"
                  required
                  className="mono"
                  value={source}
                  placeholder="https://api.ipify.org"
                  onChange={(e) =>
                    setForm({
                      ...form,
                      sources: form.sources.map((s, n) =>
                        n === i ? e.target.value : s,
                      ),
                    })
                  }
                />
                {form.sources.length > 1 && (
                  <button
                    type="button"
                    className="icon-btn"
                    aria-label="移除此检测源"
                    onClick={() =>
                      setForm({
                        ...form,
                        sources: form.sources.filter((_, n) => n !== i),
                      })
                    }
                  >
                    <X size={15} />
                  </button>
                )}
              </div>
            ))}
            {form.sources.length < 3 && (
              <button
                className="text-btn add-source"
                type="button"
                onClick={() =>
                  setForm({ ...form, sources: [...form.sources, ""] })
                }
              >
                <Plus size={14} />
                添加备用检测源
              </button>
            )}
            <p className="field-hint">
              仅支持返回纯文本 IPv4 的公开 HTTPS 服务。
            </p>
          </div>
          <div className="settings-section">
            <label className="field-label">同步策略</label>
            <div className="strategy">
              <CheckCircle2 size={16} />
              <span>先添加新 IP 规则，再撤销旧规则</span>
              <span className="fixed-tag">固定</span>
            </div>
            <div className="strategy">
              <CheckCircle2 size={16} />
              <span>仅维护带有本工具归属标记的规则</span>
            </div>
            <div className="strategy">
              <CheckCircle2 size={16} />
              <span>每次检查均核对规则，失败目标后续重试</span>
            </div>
            <p className="field-hint">
              仅同步已开启“自动同步”的目标；固定网段目标也会检查规则。
            </p>
          </div>
          <div className="settings-bottom">
            <span>
              <LockKeyhole size={13} />
              设备在线时按计划执行
            </span>
            <button className="btn primary" disabled={busy}>
              {busy ? <Spin /> : <Check size={16} />}保存设置
            </button>
          </div>
        </form>
        <aside className="card execution-card">
          <div className="card-section-title">
            <h2>执行记录</h2>
            <Clock3 size={17} />
          </div>
          <div className="next-run">
            <span
              className={`status-dot ${state.settings.enabled ? "" : "off"}`}
            />
            <span>
              {state.settings.enabled
                ? `下次检查 ${state.nextCheck ? date(state.nextCheck) : "计算中"}`
                : "计划任务尚未开启"}
            </span>
          </div>
          {records.length ? (
            <div className="timeline">
              {records.map((l) => (
                <div className="timeline-item" key={l.id}>
                  <span
                    className={`timeline-dot ${l.success ? "" : "failed"}`}
                  />
                  <small>{date(l.time)}</small>
                  <strong>{l.kind === "ip" ? "公网 IP 检查" : l.object}</strong>
                  <p className={!l.success ? "error-text" : ""}>{l.detail}</p>
                </div>
              ))}
            </div>
          ) : (
            <Empty
              icon={Clock3}
              title="暂无执行记录"
              text="检测或同步后，执行结果会显示在这里。"
            />
          )}
          <div className="execution-note">
            <Globe2 size={16} />
            <p>
              检测的是运行设备所在网络的出口 IP。
              <br />
              请让设备持续在线，并确保未进入休眠。
            </p>
          </div>
        </aside>
      </div>
    </>
  );
}
