import { useCallback, useEffect, useState } from "react";
import { Bell, CheckCircle2, Edit3, Plus, Send, Trash2 } from "lucide-react";
import { api, NotificationChannel, NotificationChannels } from "../api";
import { Empty, Modal, Spin, Toggle } from "../components/ui";
import NotificationModal from "../components/NotificationModal";

export default function Notifications() {
  const [data, setData] = useState<NotificationChannels>();
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [editing, setEditing] = useState<NotificationChannel | "new">();
  const [deleting, setDeleting] = useState<NotificationChannel>();
  const [busy, setBusy] = useState("");
  const closeEditor = useCallback(() => setEditing(undefined), []);
  const closeDelete = useCallback(() => setDeleting(undefined), []);
  const reload = useCallback(async () => {
    try {
      setData(await api<NotificationChannels>("/notification-channels"));
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "读取失败");
    }
  }, []);
  useEffect(() => {
    void reload();
  }, [reload]);
  const toggle = async (channel: NotificationChannel, enabled: boolean) => {
    setBusy(channel.id);
    setMessage("");
    try {
      await api(`/notification-channels/${channel.id}`, "PUT", {
        name: channel.name,
        type: channel.type,
        enabled,
        events: channel.events,
        templates: channel.templates,
      });
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setBusy("");
    }
  };
  return (
    <>
      <div className="page-title">
        <div>
          <div className="eyebrow muted">消息与提醒</div>
          <h1>通知渠道</h1>
          <p>订阅公网 IP 变化、规则实际变更成功及同步失败，按轮次合并推送。</p>
        </div>
        <button
          className="btn primary"
          disabled={!data}
          onClick={() => {
            setMessage("");
            setEditing("new");
          }}
        >
          <Plus size={16} />
          添加通知渠道
        </button>
      </div>
      <div className="notice info">
        <Bell size={16} />
        <span>
          首版支持飞书卡片。一轮同步向每个渠道发送一张卡片，按订阅合并 IP
          变化、实际变更成功及失败结果。单独检测时，仅推送 IP 变化。
        </span>
      </div>
      {error && (
        <div role="alert" className="notice danger">
          {error}
          <button className="text-btn" onClick={() => void reload()}>
            重新读取
          </button>
        </div>
      )}
      {message && (
        <div role="status" className="notice success">
          <CheckCircle2 size={16} />
          {message}
        </div>
      )}
      {!data ? (
        <div className="card table-loading">
          <Spin />
          正在读取通知渠道…
        </div>
      ) : data.items.length ? (
        <div className="notification-channel-grid">
          {data.items.map((channel) => (
            <section className="card notification-channel" key={channel.id}>
              <div className="notification-channel-head">
                <span className="icon-soft">
                  <Bell size={20} />
                </span>
                <div>
                  <h2>{channel.name}</h2>
                  <span className="muted">飞书 · 自定义机器人</span>
                </div>
                <Toggle
                  label={`${channel.name} 通知`}
                  checked={channel.enabled}
                  disabled={!!busy}
                  onChange={(enabled) => void toggle(channel, enabled)}
                />
              </div>
              <div className="notification-event-tags">
                {channel.events.map((event) => (
                  <span key={event} className="trigger-tag">
                    {data.events[event]}
                  </span>
                ))}
              </div>
              <p className="field-hint">
                Webhook 已加密保存 ·{" "}
                {channel.hasSignSecret ? "已配置签名校验" : "未配置签名校验"}
              </p>
              <div className="notification-channel-actions">
                <button className="btn" onClick={() => setEditing(channel)}>
                  <Edit3 size={14} />
                  编辑渠道
                </button>
                <button
                  className="text-btn"
                  onClick={() => setEditing(channel)}
                >
                  <Send size={14} />
                  模板与测试
                </button>
                <button
                  className="icon-btn danger-text"
                  aria-label={`删除渠道 ${channel.name}`}
                  onClick={() => setDeleting(channel)}
                >
                  <Trash2 size={15} />
                </button>
              </div>
            </section>
          ))}
        </div>
      ) : (
        <section className="card">
          <Empty
            icon={Bell}
            title="添加第一个通知渠道"
            text="填写飞书机器人 Webhook，选择事件并自定义卡片模板。"
            action={
              <button className="btn primary" onClick={() => setEditing("new")}>
                <Plus size={15} />
                添加通知渠道
              </button>
            }
          />
        </section>
      )}
      <p className="field-hint notification-footnote">
        通知在后台发送，失败最多尝试 3 次，结果可在“操作日志 →
        通知发送”中查看。通知失败不影响 IP 检测和云端规则同步。
      </p>
      {data && editing && (
        <NotificationModal
          channel={editing === "new" ? undefined : editing}
          metadata={data}
          onClose={closeEditor}
          onSaved={async () => {
            await reload();
            closeEditor();
            setMessage("通知渠道已保存");
          }}
        />
      )}
      {deleting && (
        <Modal title="删除通知渠道" onClose={closeDelete} busy={!!busy}>
          <div className="modal-body">
            <p>删除“{deleting.name}”及其待发送任务？历史发送日志会保留。</p>
            {error && (
              <div className="notice danger" role="alert">
                {error}
              </div>
            )}
          </div>
          <div className="modal-footer">
            <button className="btn" disabled={!!busy} onClick={closeDelete}>
              取消
            </button>
            <button
              className="btn primary"
              disabled={!!busy}
              onClick={async () => {
                setBusy(deleting.id);
                try {
                  await api(`/notification-channels/${deleting.id}`, "DELETE");
                  await reload();
                  closeDelete();
                  setMessage("通知渠道已删除");
                } catch (e) {
                  setError(e instanceof Error ? e.message : "删除失败");
                } finally {
                  setBusy("");
                }
              }}
            >
              {busy ? <Spin /> : <Trash2 size={14} />}确认删除
            </button>
          </div>
        </Modal>
      )}
    </>
  );
}
