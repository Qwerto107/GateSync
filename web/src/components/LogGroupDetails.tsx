import { useEffect, useState } from "react";
import { ChevronDown } from "lucide-react";
import { api, date, Log } from "../api";
import { Spin } from "./ui";

export default function LogGroupDetails({
  item,
  filters = "days=7",
}: {
  item: Log;
  filters?: string;
}) {
  const [open, setOpen] = useState(false);
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<Log[]>([]);
  const [total, setTotal] = useState(item.count || 0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    setError("");
    const params = new URLSearchParams(filters);
    params.delete("group");
    params.set("firstId", String(item.firstId));
    params.set("lastId", String(item.id));
    params.set("page", String(page));
    void api<{ items: Log[]; total: number }>(`/logs?${params}`)
      .then((value) => {
        if (cancelled) return;
        setItems((previous) =>
          page === 1 ? value.items : [...previous, ...value.items],
        );
        setTotal(value.total);
      })
      .catch((e) => {
        if (!cancelled) setError(e.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, item.firstId, item.id, page, retry, filters]);
  return (
    <div className="log-group">
      <div className="log-group-summary">
        <span className="muted">连续检测 {item.count} 次</span>
        <button
          type="button"
          className="text-btn"
          aria-expanded={open}
          onClick={() => {
            setOpen(!open);
            setPage(1);
            setItems([]);
          }}
        >
          {open ? "收起检测明细" : "展开检测明细"}
          <ChevronDown size={13} className={open ? "rotate" : ""} />
        </button>
      </div>
      {open && (
        <div className="log-group-details">
          <ul className="log-group-records" aria-label="公网 IP 检测明细">
            {items.map((l) => (
              <li key={l.id}>
                <time dateTime={l.time} title={l.time}>
                  {date(l.time)}
                </time>
                <span>
                  {l.trigger === "auto" ? "自动" : "手动"} · {l.detail}
                </span>
              </li>
            ))}
          </ul>
          {loading ? (
            <span className="inline muted">
              <Spin />
              正在读取明细…
            </span>
          ) : error ? (
            <div role="alert" className="error-text">
              {error}{" "}
              <button
                type="button"
                className="text-btn"
                onClick={() => setRetry((v) => v + 1)}
              >
                重试
              </button>
            </div>
          ) : (
            <div className="log-group-summary">
              <span className="muted">
                已显示 {items.length} / {total} 条
              </span>
              {items.length < total && (
                <button
                  type="button"
                  className="text-btn"
                  onClick={() => setPage((v) => v + 1)}
                >
                  加载更多
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
