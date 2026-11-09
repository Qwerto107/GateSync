import { useEffect, useRef } from "react";
import type { ReactNode } from "react";
import { Layers3, LoaderCircle, ShieldCheck, X } from "lucide-react";
import { Select } from "./Select";
export { Select } from "./Select";
import type { Region } from "../api";

export function Brand() {
  return (
    <a className="brand" href="/" aria-label="GateSync 首页">
      <span className="brand-icon">
        <ShieldCheck size={23} strokeWidth={1.9} />
      </span>
      GateSync<span className="brand-dot">.</span>
    </a>
  );
}
export function Spin() {
  return <LoaderCircle size={16} className="spin" />;
}
export function Badge({ status }: { status: string }) {
  const labels: Record<string, string> = {
    synced: "已同步",
    outdated: "IP 不一致",
    failed: "同步失败",
    pending: "待同步",
    revoked: "已撤销",
    connected: "连接正常",
    untested: "待测试",
  };
  const good = ["synced", "connected"].includes(status);
  const bad = status === "failed";
  return (
    <span className={`badge ${good ? "good" : bad ? "bad" : "warn"}`}>
      <span className="status-dot" />
      {labels[status] || status}
    </span>
  );
}
export function Empty({
  icon: Icon = Layers3,
  title,
  text,
  action,
}: {
  icon?: typeof Layers3;
  title: string;
  text: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <Icon size={26} strokeWidth={1.5} />
      </div>
      <h3>{title}</h3>
      <p>{text}</p>
      {action}
    </div>
  );
}
export function Toggle({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      className={`toggle ${checked ? "on" : ""}`}
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
    >
      <span />
    </button>
  );
}
export function Modal({
  title,
  subtitle,
  onClose,
  children,
  wide = false,
  busy = false,
}: {
  title: string;
  subtitle?: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
  busy?: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLElement>("input,select,button")?.focus();
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !busy) onClose();
      if (e.key === "Tab") {
        const nodes = [
          ...(ref.current?.querySelectorAll<HTMLElement>(
            "button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),a[href]",
          ) || []),
        ];
        const first = nodes[0],
          last = nodes.at(-1);
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        }
        if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }
    };
    document.addEventListener("keydown", handler);
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", handler);
      document.body.style.overflow = overflow;
      previous?.focus();
    };
  }, [onClose, busy]);
  return (
    <div
      className="overlay"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !busy) onClose();
      }}
    >
      <div
        className={`modal ${wide ? "wide" : ""}`}
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby="dialog-title"
      >
        <div className="modal-head">
          <div>
            <h2 id="dialog-title">{title}</h2>
            {subtitle && <p>{subtitle}</p>}
          </div>
          <button
            type="button"
            className="icon-btn"
            aria-label="关闭弹框"
            onClick={onClose}
            disabled={busy}
          >
            <X size={19} />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

export function RegionField({
  value,
  onChange,
  disabled,
  options = [],
}: {
  value: string;
  onChange: (s: string) => void;
  disabled?: boolean;
  options?: Region[];
}) {
  return (
    <label className="field">
      地域
      <Select
        label="地域"
        value={value}
        onChange={onChange}
        disabled={disabled}
        required
        searchable
        allowCustom
        placeholder="搜索地域或输入地域代码"
        options={options.map(({ id, name }) => ({
          value: id,
          label: `${name}（${id}）`,
        }))}
      />
    </label>
  );
}
