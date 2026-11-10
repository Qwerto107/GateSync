import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { Check, LockKeyhole, RotateCcw, Send } from "lucide-react";
import { api, NotificationChannel, NotificationChannels } from "../api";
import { Modal, Select, Spin, Toggle } from "./ui";

export default function NotificationModal({
  channel,
  metadata,
  onClose,
  onSaved,
}: {
  channel?: NotificationChannel;
  metadata: NotificationChannels;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [name, setName] = useState(channel?.name || "");
  const [enabled, setEnabled] = useState(channel?.enabled ?? true);
  const [events, setEvents] = useState(channel?.events || ["ip_changed"]);
  const [webhook, setWebhook] = useState("");
  const [signEnabled, setSignEnabled] = useState(
    channel?.hasSignSecret || false,
  );
  const [secret, setSecret] = useState("");
  const [templates, setTemplates] = useState({
    ...metadata.defaultTemplates,
    ...channel?.templates,
  });
  const [event, setEvent] = useState("ip_changed");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [confirmTest, setConfirmTest] = useState(false);
  const [preview, setPreview] = useState<Record<string, unknown>>();
  const [previewError, setPreviewError] = useState("");
  const template = templates[event];
  useEffect(() => {
    let cancelled = false;
    setPreview(undefined);
    setPreviewError("");
    const timer = setTimeout(() => {
      void api<{ card: Record<string, unknown> }>(
        "/notification-channels/preview",
        "POST",
        { event, templates: { [event]: template } },
      )
        .then((value) => {
          if (!cancelled) setPreview(value.card);
        })
        .catch((e) => {
          if (!cancelled) setPreviewError(e.message);
        });
    }, 300);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [event, template]);
  const input = () => ({
    id: channel?.id || "",
    name,
    type: "feishu",
    enabled,
    events,
    templates,
    webhook,
    signSecret: signEnabled ? secret : "",
    clearSignSecret: !signEnabled,
  });
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy("save");
    setError("");
    setMessage("");
    try {
      await api(
        channel
          ? `/notification-channels/${channel.id}`
          : "/notification-channels",
        channel ? "PUT" : "POST",
        input(),
      );
      await onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setBusy("");
    }
  };
  return (
    <Modal
      title={channel ? "编辑通知渠道" : "添加通知渠道"}
      subtitle="飞书自定义机器人 · 卡片 JSON 2.0"
      onClose={onClose}
      busy={!!busy}
      wide
    >
      <form onSubmit={save}>
        <fieldset className="modal-body notification-fields" disabled={!!busy}>
          <div className="form-grid">
            <label className="field">
              渠道名称
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                maxLength={60}
                placeholder="例如：办公室飞书群"
              />
            </label>
            <label className="field">
              渠道类型
              <Select
                label="渠道类型"
                value="feishu"
                onChange={() => {}}
                options={[{ value: "feishu", label: "飞书自定义机器人" }]}
              />
            </label>
          </div>
          <div className="notification-enable">
            <div>
              <strong>启用通知</strong>
              <p className="field-hint">保存后，仅向此渠道发送已选事件</p>
            </div>
            <Toggle
              label="启用此通知渠道"
              checked={enabled}
              onChange={setEnabled}
              disabled={!!busy}
            />
          </div>
          <label className="field">
            Webhook 地址
            <input
              type="password"
              autoComplete="new-password"
              value={webhook}
              required={!channel?.hasWebhook}
              maxLength={1024}
              onChange={(e) => setWebhook(e.target.value)}
              placeholder={
                channel?.hasWebhook
                  ? "已保存，留空保留原地址"
                  : "https://open.feishu.cn/open-apis/bot/v2/hook/…"
              }
            />
            <span className="field-hint">
              <LockKeyhole size={12} />
              加密保存，不回显；仅支持飞书官方机器人 Webhook
            </span>
          </label>
          <div className="notification-enable">
            <label className="check-label">
              <input
                type="checkbox"
                checked={signEnabled}
                onChange={(e) => setSignEnabled(e.target.checked)}
              />
              机器人已开启签名校验
            </label>
          </div>
          {signEnabled && (
            <label className="field">
              签名密钥
              <input
                type="password"
                autoComplete="new-password"
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
                maxLength={512}
                required={!channel?.hasSignSecret}
                placeholder={
                  channel?.hasSignSecret
                    ? "已保存，留空保留原密钥"
                    : "填写飞书机器人设置中的签名密钥"
                }
              />
            </label>
          )}
          <div className="field">
            <span>通知事件</span>
            <div className="notification-event-options">
              {Object.entries(metadata.events).map(([kind, label]) => (
                <label className="check-label" key={kind}>
                  <input
                    type="checkbox"
                    checked={events.includes(kind)}
                    onChange={(e) =>
                      setEvents(
                        e.target.checked
                          ? [...events, kind]
                          : events.filter((v) => v !== kind),
                      )
                    }
                  />
                  {label}
                </label>
              ))}
            </div>
            <span className="field-hint">
              同步成功仅在实际修改规则后通知。一轮同步按订阅合并为一张卡片；首次检测、IP
              无变化及规则无需修改时不产生对应事件。
            </span>
          </div>
          <div className="notification-template-header">
            <h3>卡片模板</h3>
            <Select
              label="编辑模板事件"
              value={event}
              onChange={(v) => {
                setEvent(v);
                setConfirmTest(false);
                setMessage("");
              }}
              options={Object.entries(metadata.templateNames).map(
                ([value, label]) => ({ value, label }),
              )}
            />
            <button
              type="button"
              className="text-btn"
              onClick={() =>
                setTemplates({
                  ...templates,
                  [event]: metadata.defaultTemplates[event],
                })
              }
            >
              <RotateCcw size={13} />
              恢复默认
            </button>
          </div>
          <label className="field">
            卡片 JSON
            <textarea
              aria-label="飞书卡片 JSON 模板"
              className="mono notification-template"
              rows={12}
              spellCheck={false}
              value={template}
              onChange={(e) => {
                setTemplates({ ...templates, [event]: e.target.value });
                setConfirmTest(false);
              }}
            />
            <span className="field-hint">
              单一事件和合并结果可分别编辑，最多 16 KiB。填写 card
              对象本身，不包含 msg_type 外层。变量只替换字符串值。
            </span>
          </label>
          <details className="notification-variables">
            <summary>可用变量</summary>
            <div>
              {metadata.variables.map((v) => (
                <code key={v}>{`{{${v}}}`}</code>
              ))}
            </div>
            <p className="field-hint">
              old_ip / new_ip：旧 / 新 IP；time /
              timezone：时间与任务时区；target_name / resource_id / region /
              error：目标及错误；success_count / failure_count：成功变更 /
              失败目标数；summary：目标结果摘要；ip_summary：IP
              变化摘要；trigger：触发方式；site_url：站点地址。
            </p>
          </details>
          <div className="notification-preview">
            <h3>示例预览</h3>
            {previewError ? (
              <div className="notice danger" role="alert">
                {previewError}
              </div>
            ) : preview ? (
              <>
                <CardPreview card={preview} />
                <p className="field-hint">
                  使用示例数据，不会发送消息。此处展示标题和 Markdown
                  文本，完整效果以飞书实际接收为准。
                </p>
                <details>
                  <summary>查看渲染后的 JSON</summary>
                  <pre className="mono">{JSON.stringify(preview, null, 2)}</pre>
                </details>
              </>
            ) : (
              <span className="inline muted">
                <Spin />
                正在校验模板…
              </span>
            )}
          </div>
          {error && (
            <div className="notice danger" role="alert">
              {error}
            </div>
          )}
          {message && (
            <div className="notice success" role="status">
              {message}
            </div>
          )}
          {confirmTest && (
            <div className="notice info notification-test-confirm">
              <span>
                将向当前配置的飞书群实际发送一张“{metadata.templateNames[event]}
                ”测试卡片，使用示例数据，不保存表单。
              </span>
              <div className="inline">
                <button
                  type="button"
                  className="btn"
                  onClick={() => setConfirmTest(false)}
                >
                  取消
                </button>
                <button
                  type="button"
                  className="btn primary"
                  disabled={!!busy}
                  onClick={async () => {
                    if (
                      !name.trim() ||
                      (!webhook && !channel?.hasWebhook) ||
                      (signEnabled && !secret && !channel?.hasSignSecret)
                    ) {
                      setError(
                        "请先填写渠道名称、Webhook 和已启用的签名密钥。",
                      );
                      return;
                    }
                    setBusy("test");
                    setError("");
                    setMessage("");
                    try {
                      const result = await api<{ message: string }>(
                        "/notification-channels/test",
                        "POST",
                        { ...input(), event },
                      );
                      setMessage(result.message);
                      setConfirmTest(false);
                    } catch (e) {
                      setError(e instanceof Error ? e.message : "测试发送失败");
                    } finally {
                      setBusy("");
                    }
                  }}
                >
                  {busy === "test" ? <Spin /> : <Send size={14} />}确认发送测试
                </button>
              </div>
            </div>
          )}
        </fieldset>
        <div className="modal-footer">
          <button
            type="button"
            className="btn"
            disabled={!!busy || !!previewError}
            onClick={() => {
              setConfirmTest(true);
              setError("");
              setMessage("");
            }}
          >
            <Send size={14} />
            测试发送
          </button>
          <button
            type="button"
            className="btn"
            disabled={!!busy}
            onClick={onClose}
          >
            取消
          </button>
          <button className="btn primary" disabled={!!busy}>
            {busy === "save" ? <Spin /> : <Check size={14} />}保存渠道
          </button>
        </div>
      </form>
    </Modal>
  );
}

function cardText(value: unknown): string {
  return typeof value === "string" ? value : "";
}
function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}
function formattedText(text: string): ReactNode[] {
  // 仅渲染明确支持的文字标记；模板中的任意 HTML 不进入 DOM。
  const pattern =
    /<font color=['"]?(red|green|grey)['"]?>([\s\S]*?)<\/font>|~~([^~]+)~~|\*\*([^*]+)\*\*/g;
  const nodes: ReactNode[] = [];
  let cursor = 0;
  for (const match of text.matchAll(pattern)) {
    nodes.push(text.slice(cursor, match.index));
    if (match[1])
      nodes.push(
        <span key={match.index} className={`card-text-${match[1]}`}>
          {formattedText(match[2])}
        </span>,
      );
    else if (match[3]) nodes.push(<del key={match.index}>{match[3]}</del>);
    else nodes.push(<strong key={match.index}>{match[4]}</strong>);
    cursor = match.index! + match[0].length;
  }
  nodes.push(text.slice(cursor));
  return nodes;
}
function CardPreview({ card }: { card: Record<string, unknown> }) {
  const header = record(card.header),
    body = record(card.body);
  const elements = Array.isArray(body.elements) ? body.elements : [];
  return (
    <div
      className={`feishu-card-preview ${header.template === "red" ? "failed" : header.template === "green" ? "succeeded" : ""}`}
    >
      <div className="feishu-card-heading">
        {cardText(record(header.title).content) || "卡片预览"}
      </div>
      <div className="feishu-card-content">
        {elements.map((value, i) => {
          const element = record(value);
          return element.tag === "markdown" ? (
            <div key={i}>{formattedText(cardText(element.content))}</div>
          ) : null;
        })}
      </div>
    </div>
  );
}
