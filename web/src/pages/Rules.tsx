import { useEffect, useRef, useState } from "react";
import {
  ArrowUp,
  ArrowDown,
  GripVertical,
  Copy,
  Globe2,
  MoreHorizontal,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  Trash2,
  TriangleAlert,
  X,
} from "lucide-react";
import { date, State, Target } from "../api";
import { Spin, Badge, Empty, Toggle } from "../components/ui";

export default function Rules({
  state,
  busy,
  syncing,
  onAdd,
  onEdit,
  onDelete,
  onDetect,
  onSync,
  onSyncAll,
  onToggle,
  onReorder,
}: {
  state: State;
  busy: Set<string>;
  syncing: boolean;
  onAdd: () => void;
  onEdit: (t: Target) => void;
  onDelete: (t: Target) => void;
  onDetect: () => void;
  onSync: (id: string) => void;
  onSyncAll: () => void;
  onToggle: (t: Target) => void;
  onReorder: (ids: string[]) => Promise<void>;
}) {
  const drag = useRef<{
    id: string;
    x: number;
    y: number;
    over: string | null;
    moved: boolean;
  } | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const [reordering, setReordering] = useState(false);
  const reorder = async (from: string, to: string) => {
    if (reordering || from === to) return;
    const ids = state.targets.map((t) => t.id);
    const index = ids.indexOf(from),
      next = ids.indexOf(to);
    if (index < 0 || next < 0) return;
    ids.splice(index, 1);
    ids.splice(next, 0, from);
    setReordering(true);
    try {
      await onReorder(ids);
    } finally {
      setReordering(false);
    }
  };
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("all");
  const targets = state.targets.filter(
    (t) =>
      (!search ||
        `${t.name} ${t.region} ${t.groupId} ${t.groupName} ${t.note} ${t.fixedIps?.map((ip) => `${ip.cidr} ${ip.name || ""}`).join(" ") || t.fixedCidr} ${t.ports.map((p) => p.value)} ${state.accounts.find((a) => a.id === t.accountId)?.name}`
          .toLowerCase()
          .includes(search.toLowerCase())) &&
      (filter === "all" || t.status !== "synced"),
  );
  const synced = state.targets.filter((t) => t.status === "synced").length;
  const pending = state.targets.length - synced;
  const source = state.ip.source
    ? new URL(state.ip.source).hostname
    : "尚未检测";
  return (
    <>
      <div className="page-title">
        <div>
          <div className="eyebrow muted">安全组管理</div>
          <h1>
            放行规则<span className="count-pill">{state.targets.length}</span>
          </h1>
          <p>连接随 IP 而变，让放行规则保持最新。</p>
        </div>
        <div className="page-caption">
          <span className="provider-logo">T</span>腾讯云已接入
          <span className="small-dot" />
          更多云厂商即将支持
        </div>
      </div>
      <div className="ip-overview">
        <section className="card current-ip">
          <div className="section-label">
            <span className="icon-soft">
              <Globe2 size={18} />
            </span>
            当前公网 IP<span className="live-tag">IPv4</span>
          </div>
          <div className="ip-value mono">
            {state.ip.ip || "等待检测"}
            {state.ip.ip && (
              <button
                className="icon-btn"
                aria-label="复制公网 IP"
                onClick={() =>
                  void navigator.clipboard
                    .writeText(state.ip.ip)
                    .catch(() => {})
                }
              >
                <Copy size={15} />
              </button>
            )}
          </div>
          <div className="ip-bottom">
            <span>
              {state.ip.checkedAt
                ? `检测于 ${date(state.ip.checkedAt)} · ${source}`
                : "检测运行设备所在网络的出口 IP"}
            </span>
            <button
              className="text-btn"
              onClick={onDetect}
              disabled={busy.has("detect")}
            >
              {busy.has("detect") ? <Spin /> : <RefreshCw size={13} />}重新检测
            </button>
          </div>
        </section>
        <section
          className={`card sync-overview ${pending ? "needs-sync" : ""}`}
        >
          <div className="sync-state-icon">
            {pending ? <RefreshCw size={23} /> : <ShieldCheck size={25} />}
          </div>
          <div className="sync-copy">
            <h2>
              {state.targets.length === 0
                ? "从一个放行目标开始"
                : pending
                  ? `${pending} 个目标待同步`
                  : "所有目标已保持同步"}
            </h2>
            <p>
              {state.targets.length
                ? `${state.targets.length} 个目标中 ${synced} 个规则与配置一致`
                : "关联安全组和端口，后续自动跟随公网 IP"}
            </p>
            <span className="schedule-hint">
              <span
                className={`status-dot ${state.settings.enabled ? "" : "off"}`}
              />
              {state.settings.enabled
                ? `自动同步已开启${state.nextCheck ? ` · 下次检查 ${date(state.nextCheck)}` : ""}`
                : "自动同步未开启，可在计划任务中设置"}
            </span>
          </div>
          <button
            className="btn primary sync-all"
            disabled={!state.targets.length || syncing}
            onClick={onSyncAll}
          >
            {busy.has("sync-all") ? <Spin /> : <RefreshCw size={15} />}全部同步
          </button>
        </section>
      </div>
      {state.ip.error && (
        <div className="notice danger">
          <TriangleAlert size={16} />
          {state.ip.error}
        </div>
      )}
      <section className="card data-card">
        <div className="card-toolbar">
          <div className="table-heading">
            <h2>放行目标</h2>
            <span className="sort-hint">
              拖动手柄或上移、下移排序{reordering ? " · 保存中…" : ""}
            </span>
            <div className="filter-tabs">
              <button
                className={filter === "all" ? "selected" : ""}
                onClick={() => setFilter("all")}
              >
                全部<span>{state.targets.length}</span>
              </button>
              <button
                className={filter === "pending" ? "selected" : ""}
                onClick={() => setFilter("pending")}
              >
                待同步<span>{pending}</span>
              </button>
            </div>
          </div>
          <div className="toolbar-actions">
            <div className="search">
              <Search size={15} />
              <input
                aria-label="搜索放行目标"
                placeholder="搜索名称 / 资源 / 端口"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              {search && (
                <button
                  className="icon-btn"
                  onClick={() => setSearch("")}
                  aria-label="清空搜索"
                >
                  <X size={13} />
                </button>
              )}
            </div>
            <button className="btn primary" onClick={onAdd}>
              <Plus size={16} />
              添加放行目标
            </button>
          </div>
        </div>
        {targets.length ? (
          <div className="table-scroll">
            <table className="targets-table">
              <thead>
                <tr>
                  <th className="sort-column">排序</th>
                  <th>目标名称 / 云端资源</th>
                  <th>云账号 / 地域</th>
                  <th>放行端口</th>
                  <th>规则中的 IP</th>
                  <th>状态</th>
                  <th>自动同步</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {targets.map((t, index) => {
                  const account = state.accounts.find(
                    (a) => a.id === t.accountId,
                  );
                  return (
                    <tr
                      key={t.id}
                      data-target-id={t.id}
                      className={dropTarget === t.id ? "drop-target" : ""}
                    >
                      <td className="sort-column">
                        <div className="sort-controls">
                          <button
                            className="icon-btn drag-handle"
                            aria-label={`拖动排序 ${t.name || t.groupName || t.groupId}`}
                            disabled={reordering}
                            onPointerDown={(e) => {
                              if (e.button !== 0 || reordering) return;
                              e.preventDefault();
                              e.currentTarget.setPointerCapture(e.pointerId);
                              drag.current = {
                                id: t.id,
                                x: e.clientX,
                                y: e.clientY,
                                over: null,
                                moved: false,
                              };
                            }}
                            onPointerMove={(e) => {
                              const current = drag.current;
                              if (
                                !current ||
                                (!current.moved &&
                                  Math.hypot(
                                    e.clientX - current.x,
                                    e.clientY - current.y,
                                  ) < 5)
                              )
                                return;
                              current.moved = true;
                              // 使用指针捕获，同时兼容鼠标和触屏；命中行决定插入位置。
                              const row = document
                                .elementFromPoint(e.clientX, e.clientY)
                                ?.closest<HTMLTableRowElement>(
                                  "tr[data-target-id]",
                                );
                              current.over = row?.dataset.targetId || null;
                              setDropTarget(current.over);
                            }}
                            onPointerUp={(e) => {
                              const current = drag.current;
                              drag.current = null;
                              e.currentTarget.releasePointerCapture(
                                e.pointerId,
                              );
                              setDropTarget(null);
                              if (current?.over)
                                void reorder(current.id, current.over);
                            }}
                            onPointerCancel={() => {
                              drag.current = null;
                              setDropTarget(null);
                            }}
                          >
                            <GripVertical size={16} />
                          </button>
                          <span className="sort-arrows">
                            <button
                              className="icon-btn"
                              aria-label={`上移 ${t.name || t.groupName || t.groupId}`}
                              disabled={reordering || index === 0}
                              onClick={() =>
                                void reorder(t.id, targets[index - 1].id)
                              }
                            >
                              <ArrowUp size={12} />
                            </button>
                            <button
                              className="icon-btn"
                              aria-label={`下移 ${t.name || t.groupName || t.groupId}`}
                              disabled={
                                reordering || index === targets.length - 1
                              }
                              onClick={() =>
                                void reorder(t.id, targets[index + 1].id)
                              }
                            >
                              <ArrowDown size={12} />
                            </button>
                          </span>
                        </div>
                      </td>
                      <td>
                        <strong>{t.name || t.groupName || t.groupId}</strong>
                        <small
                          className="resource-detail"
                          title={`${t.groupName || t.groupId} · ${t.groupId} · ${t.resourceType === "lighthouse" ? "Lighthouse 防火墙" : "VPC 安全组"}`}
                        >
                          {t.name && t.groupName && (
                            <span>{t.groupName} · </span>
                          )}
                          <span className="mono">{t.groupId}</span>
                        </small>
                        {t.note && (
                          <small className="note-text" title={t.note}>
                            {t.note}
                          </small>
                        )}
                      </td>
                      <td>
                        <div className="account-cell">
                          <span className="provider-logo small">T</span>
                          <div>
                            <strong>{account?.name || "未知账号"}</strong>
                            <small className="mono">{t.region}</small>
                          </div>
                        </div>
                      </td>
                      <td>
                        <div
                          className="port-list"
                          title={t.ports
                            .map((p) => `${p.protocol} ${p.value}`)
                            .join("、")}
                        >
                          {t.ports.slice(0, 2).map((p) => (
                            <span
                              className="port-tag mono"
                              key={`${p.protocol}-${p.value}`}
                            >
                              <span>{p.protocol}</span>
                              {p.value}
                            </span>
                          ))}
                          {t.ports.length > 2 && (
                            <details className="extra-values">
                              <summary>另 {t.ports.length - 2} 项</summary>
                              {t.ports.slice(2).map((p) => (
                                <span key={`${p.protocol}-${p.value}`}>
                                  {p.protocol} {p.value}
                                </span>
                              ))}
                            </details>
                          )}
                        </div>
                      </td>
                      <td>
                        <div className="target-ip-list">
                          {(
                            t.currentCidrs ??
                            (t.currentCidr ? [t.currentCidr] : [])
                          )
                            .slice(0, 2)
                            .map((cidr) => (
                              <span className="mono ip-cell" key={cidr}>
                                {cidr}
                              </span>
                            ))}
                          {(t.currentCidrs?.length || 0) > 2 && (
                            <details className="extra-values">
                              <summary>
                                另 {t.currentCidrs!.length - 2} 个 IP
                              </summary>
                              {t.currentCidrs!.slice(2).map((cidr) => (
                                <span className="mono" key={cidr}>
                                  {cidr}
                                </span>
                              ))}
                            </details>
                          )}
                          {!(t.currentCidrs?.length || t.currentCidr) && (
                            <span>—</span>
                          )}
                        </div>
                        <small>
                          {t.ipMode === "dynamic" ? "跟随公网 IP" : "固定 IP"}
                          {!!t.fixedIps?.length &&
                            ` · 自定义 ${t.fixedIps.length} 项`}
                        </small>
                      </td>
                      <td>
                        <Badge status={t.status} />
                        {t.error ? (
                          <details className="target-error error-text">
                            <summary title={t.error}>查看错误详情</summary>
                            <p>{t.error}</p>
                          </details>
                        ) : (
                          <small>
                            {t.lastSync ? date(t.lastSync) : "尚未同步"}
                          </small>
                        )}
                      </td>
                      <td>
                        <Toggle
                          checked={t.autoSync}
                          onChange={() => onToggle(t)}
                          label={`${t.groupName || t.groupId} 自动同步`}
                          disabled={busy.has(`toggle-${t.id}`)}
                        />
                      </td>
                      <td>
                        <div className="row-actions">
                          <button
                            className="text-btn"
                            onClick={() => onSync(t.id)}
                            disabled={syncing}
                          >
                            {busy.has(`sync-${t.id}`) ? (
                              <Spin />
                            ) : (
                              <RefreshCw size={13} />
                            )}
                            {t.status === "failed" ? "重试" : "同步"}
                          </button>
                          <RowMenu
                            onEdit={() => onEdit(t)}
                            onDelete={() => onDelete(t)}
                          />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty
            title={
              search || filter !== "all"
                ? "没有匹配的放行目标"
                : "添加你的第一个放行目标"
            }
            text={
              search || filter !== "all"
                ? "试试调整搜索内容或筛选条件。"
                : "选择安全组或 Lighthouse 实例，配置需要放行的端口。"
            }
            action={
              !search && filter === "all" ? (
                <button className="btn" onClick={onAdd}>
                  <Plus size={15} />
                  添加放行目标
                </button>
              ) : undefined
            }
          />
        )}
        <div className="table-footer">
          <span>
            <span className="provider-logo tiny">T</span>腾讯云 · IPv4 入站规则
          </span>
          <span>共 {targets.length} 个目标</span>
        </div>
      </section>
      <div className="policy-note">
        <ShieldCheck size={17} />
        <p>
          同步时先添加新 IP 规则，再删除旧规则；仅处理本工具创建并标记的规则。
          <br />
          <span>安全组中的其他规则保持原样。</span>
        </p>
      </div>
    </>
  );
}
function RowMenu({
  onEdit,
  onDelete,
}: {
  onEdit: () => void;
  onDelete: () => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", escape);
    };
  }, [open]);
  return (
    <div className="row-menu-wrap" ref={ref}>
      <button
        className="icon-btn"
        aria-label="更多目标操作"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <MoreHorizontal size={18} />
      </button>
      {open && (
        <div className="row-menu">
          <button
            onClick={() => {
              setOpen(false);
              onEdit();
            }}
          >
            <Pencil size={14} />
            编辑目标
          </button>
          <button
            className="error-text"
            onClick={() => {
              setOpen(false);
              onDelete();
            }}
          >
            <Trash2 size={14} />
            删除目标
          </button>
        </div>
      )}
    </div>
  );
}
