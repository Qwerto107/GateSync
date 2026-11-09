import { useEffect, useState } from "react";
import {
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Download,
  ListFilter,
  RefreshCw,
  Search,
  X,
} from "lucide-react";
import { api, date, kinds, Log } from "../api";
import { Spin, Empty, Select } from "../components/ui";

export default function Logs() {
  const [filter, setFilter] = useState({
    days: "7",
    kind: "",
    result: "",
    q: "",
  });
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<Log[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<number | null>(null);
  const [refresh, setRefresh] = useState(0);
  const params = new URLSearchParams({
    ...filter,
    page: String(page),
  }).toString();
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    const timer = setTimeout(() => {
      void api<{ items: Log[]; total: number }>(`/logs?${params}`)
        .then((v) => {
          if (!cancelled) {
            setItems(v.items);
            setTotal(v.total);
            setError("");
          }
        })
        .catch((e) => {
          if (!cancelled) setError(e.message);
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, 200);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [params, refresh]);
  const update = (key: string, value: string) => {
    setFilter((v) => ({ ...v, [key]: value }));
    setPage(1);
  };
  return (
    <>
      <div className="page-title">
        <div>
          <div className="eyebrow muted">可追溯的每次变更</div>
          <h1>操作日志</h1>
          <p>查看检测、同步和账号操作，排查每一次异常。</p>
        </div>
        <a className="btn" href={`/api/logs/export?${params}`} download>
          <Download size={16} />
          导出 CSV
        </a>
      </div>
      <section className="card data-card">
        <div className="card-toolbar log-toolbar">
          <div className="inline">
            <Select
              label="日志时间范围"
              value={filter.days}
              onChange={(v) => update("days", v)}
              options={[
                { value: "7", label: "最近 7 天" },
                { value: "30", label: "最近 30 天" },
                { value: "90", label: "最近 90 天" },
              ]}
            />
            <Select
              label="日志类型"
              value={filter.kind}
              onChange={(v) => update("kind", v)}
              options={[
                { value: "", label: "全部类型" },
                ...Object.entries(kinds).map(([value, label]) => ({
                  value,
                  label,
                })),
              ]}
            />
            <Select
              label="日志结果"
              value={filter.result}
              onChange={(v) => update("result", v)}
              options={[
                { value: "", label: "全部结果" },
                { value: "success", label: "成功" },
                { value: "failed", label: "失败" },
              ]}
            />
          </div>
          <div className="toolbar-actions">
            <div className="search">
              <Search size={15} />
              <input
                aria-label="搜索日志"
                placeholder="搜索对象 / 详情"
                value={filter.q}
                onChange={(e) => update("q", e.target.value)}
              />
            </div>
            <button
              className="icon-btn"
              onClick={() => setRefresh((v) => v + 1)}
              aria-label="刷新日志"
            >
              {loading ? <Spin /> : <RefreshCw size={16} />}
            </button>
          </div>
        </div>
        {error ? (
          <div className="notice danger" role="alert">
            {error}
          </div>
        ) : loading ? (
          <div className="table-loading">
            <Spin />
            正在读取日志…
          </div>
        ) : items.length ? (
          <div className="table-scroll">
            <table className="logs-table">
              <thead>
                <tr>
                  <th>时间</th>
                  <th>类型</th>
                  <th>触发</th>
                  <th>对象</th>
                  <th>详情</th>
                  <th>结果</th>
                </tr>
              </thead>
              <tbody>
                {items.map((l) => (
                  <LogRow
                    key={l.id}
                    item={l}
                    expanded={expanded === l.id}
                    onExpand={() =>
                      setExpanded(expanded === l.id ? null : l.id)
                    }
                  />
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty
            icon={ListFilter}
            title="没有符合条件的日志"
            text="调整筛选条件，或完成一次检测与同步后再来查看。"
          />
        )}
        <div className="table-footer">
          <span>共 {total} 条，每页 20 条 · 导出最多 10,000 条</span>
          <div className="pagination">
            <button
              className="icon-btn"
              onClick={() => setPage((v) => v - 1)}
              disabled={page <= 1}
              aria-label="上一页"
            >
              <ChevronLeft size={16} />
            </button>
            {page} / {Math.max(1, Math.ceil(total / 20))}
            <button
              className="icon-btn"
              onClick={() => setPage((v) => v + 1)}
              disabled={page * 20 >= total}
              aria-label="下一页"
            >
              <ChevronRight size={16} />
            </button>
          </div>
        </div>
      </section>
    </>
  );
}
function LogRow({
  item: l,
  expanded,
  onExpand,
}: {
  item: Log;
  expanded: boolean;
  onExpand: () => void;
}) {
  return (
    <>
      <tr>
        <td className="nowrap muted">{date(l.time)}</td>
        <td>{kinds[l.kind] || l.kind}</td>
        <td>
          <span className={`trigger-tag ${l.trigger === "auto" ? "auto" : ""}`}>
            {l.trigger === "auto" ? "自动" : "手动"}
          </span>
        </td>
        <td className="log-object">{l.object}</td>
        <td className="log-detail">
          <span className={!l.success ? "error-text" : ""}>{l.detail}</span>
          {l.changes.length > 0 && (
            <button className="text-btn change-button" onClick={onExpand}>
              {expanded ? "收起变更" : `${l.changes.length} 条变更`}
              <ChevronDown size={13} className={expanded ? "rotate" : ""} />
            </button>
          )}
        </td>
        <td>
          <span className={`badge ${l.success ? "good" : "bad"}`}>
            {l.success ? <Check size={12} /> : <X size={12} />}
            {l.success ? "成功" : "失败"}
          </span>
        </td>
      </tr>
      {expanded && (
        <tr className="changes-row">
          <td colSpan={6}>
            <div className="changes-list mono">
              {l.changes.map((c, i) => (
                <div
                  key={i}
                  className={c.startsWith("+") ? "added" : "removed"}
                >
                  {c}
                </div>
              ))}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}
