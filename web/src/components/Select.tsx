import { useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Check, ChevronDown } from "lucide-react";

type Option = { value: string; label: string };

export function Select({
  value,
  onChange,
  options,
  label,
  disabled,
  required,
  searchable = false,
  allowCustom = false,
  placeholder = "请选择",
}: {
  value: string;
  onChange: (value: string) => void;
  options: Option[];
  label: string;
  disabled?: boolean;
  required?: boolean;
  searchable?: boolean;
  allowCustom?: boolean;
  placeholder?: string;
}) {
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const [position, setPosition] = useState({
    left: 0,
    top: 0,
    width: 0,
    maxHeight: 260,
  });
  const selected = options.find((o) => o.value === value);
  const filtered = options.filter((o) =>
    `${o.label} ${o.value}`.toLowerCase().includes(query.toLowerCase()),
  );
  const items =
    allowCustom &&
    query.trim() &&
    !options.some((o) => o.value === query.trim())
      ? [
          ...filtered,
          { value: query.trim(), label: `使用手动输入：${query.trim()}` },
        ]
      : filtered;
  const choose = (next: string) => {
    if (root.current?.querySelector(":disabled")) return;
    onChange(next);
    setOpen(false);
    setQuery("");
  };
  useEffect(() => {
    if (!open) return;
    // 挂到页面顶层，避免弹窗和表格的滚动容器裁剪选项。
    const place = () => {
      const rect = root.current!.getBoundingClientRect();
      const below = window.innerHeight - rect.bottom - 12;
      const above = rect.top - 12;
      const height = Math.min(260, Math.max(below, above));
      const upwards = below < 180 && above > below;
      setPosition({
        left: rect.left,
        width: rect.width,
        top: upwards ? rect.top - 6 : rect.bottom + 6,
        maxHeight: height,
      });
      if (panel.current)
        panel.current.style.transform = upwards ? "translateY(-100%)" : "";
    };
    const outside = (e: MouseEvent) => {
      if (
        !root.current?.contains(e.target as Node) &&
        !panel.current?.contains(e.target as Node)
      )
        setOpen(false);
    };
    place();
    document.addEventListener("mousedown", outside);
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      document.removeEventListener("mousedown", outside);
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [open]);
  useEffect(() => {
    if (!open || !position.width) return;
    panel.current
      ?.querySelector(`[data-index="${active}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [active, open, position.width]);
  const expand = () => {
    setQuery("");
    setActive(
      Math.max(
        0,
        options.findIndex((o) => o.value === value),
      ),
    );
    setOpen(true);
  };
  const common = {
    role: "combobox" as const,
    "aria-label": label,
    "aria-expanded": open,
    "aria-controls": open ? id : undefined,
    "aria-activedescendant":
      open && items[active] ? `${id}-${active}` : undefined,
    "aria-required": required,
    disabled,
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === "Escape" && open) {
        e.preventDefault();
        e.stopPropagation();
        setOpen(false);
      } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        if (!open) expand();
        else
          setActive((i) =>
            Math.max(
              0,
              Math.min(items.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)),
            ),
          );
      } else if (e.key === "Enter" && open) {
        e.preventDefault();
        if (items[active]) choose(items[active].value);
      } else if (e.key === "Tab") setOpen(false);
    },
  };
  return (
    <div className="custom-select" ref={root}>
      {searchable ? (
        <input
          {...common}
          autoComplete="off"
          required={required}
          placeholder={placeholder}
          value={open ? query : selected?.label || value}
          onFocus={expand}
          onClick={() => {
            if (!open) expand();
          }}
          onChange={(e) => {
            setQuery(e.target.value);
            setActive(0);
            setOpen(true);
          }}
          onBlur={() => setOpen(false)}
        />
      ) : (
        <button
          {...common}
          type="button"
          className="select-trigger"
          onClick={() => (open ? setOpen(false) : expand())}
        >
          <span>{selected?.label || value || placeholder}</span>
        </button>
      )}
      <ChevronDown size={14} className="select-chevron" />
      {open &&
        !disabled &&
        createPortal(
          <div
            className="select-options"
            id={id}
            role="listbox"
            aria-label={label}
            ref={panel}
            style={position}
            onMouseDown={(e) => e.preventDefault()}
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
            }}
          >
            {items.map((option, index) => (
              <div
                id={`${id}-${index}`}
                role="option"
                aria-selected={option.value === value}
                data-index={index}
                className={`select-option ${active === index ? "active" : ""}`}
                key={option.value}
                onMouseEnter={() => setActive(index)}
                onClick={() => choose(option.value)}
              >
                <span>{option.label}</span>
                {option.value === value && <Check size={14} />}
              </div>
            ))}
            {!items.length && <div className="select-empty">没有匹配选项</div>}
          </div>,
          document.body,
        )}
    </div>
  );
}
