import { useCallback, useEffect, useRef, useState } from "react";
import { CalendarDays, ChevronLeft, ChevronRight } from "lucide-react";
import { Modal, Select } from "./ui";
import {
  localDate,
  localTime,
  LogTimeRange,
  logRangePresets,
  parseLocalDateTime,
  rangeLabel,
  recentLogRange,
} from "./logTimeRange";

export default function LogTimeRangePicker({
  value,
  onChange,
}: {
  value: LogTimeRange;
  onChange: (range: LogTimeRange) => void;
}) {
  const [open, setOpen] = useState(false);
  const [start, setStart] = useState({ day: "", clock: "" });
  const [end, setEnd] = useState({ day: "", clock: "" });
  const [error, setError] = useState("");
  const close = useCallback(() => setOpen(false), []);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const from = parseLocalDateTime(start.day, start.clock);
  const to = parseLocalDateTime(end.day, end.clock);
  const highlightedRange =
    from && to && from < to ? { start: start.day, end: end.day } : undefined;
  return (
    <div className="log-time-filter">
      <button
        type="button"
        className="btn log-range-trigger"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={`日志时间范围：${rangeLabel(value)}`}
        onClick={() => {
          const from = new Date(value.from),
            to = new Date(value.to);
          setStart({ day: localDate(from), clock: localTime(from) });
          setEnd({ day: localDate(to), clock: localTime(to) });
          setError("");
          setOpen(true);
        }}
      >
        <CalendarDays size={16} />
        <span>{rangeLabel(value)}</span>
      </button>
      <div className="log-range-presets" aria-label="快捷时间范围">
        {logRangePresets.map((p) => (
          <button
            type="button"
            key={p.hours}
            className={value.hours === p.hours ? "selected" : ""}
            aria-pressed={value.hours === p.hours}
            onClick={() => onChange(recentLogRange(p.hours))}
          >
            {p.label}
          </button>
        ))}
      </div>
      <span className="field-hint">本地时区 · {timezone}</span>
      {open && (
        <Modal
          title="选择日志时间范围"
          subtitle={`按浏览器本地时区 ${timezone} 选择，结束时间不包含在范围内。`}
          onClose={close}
          wide
        >
          <form
            onSubmit={(e) => {
              e.preventDefault();
              const from = parseLocalDateTime(start.day, start.clock);
              const to = parseLocalDateTime(end.day, end.clock);
              if (!from || !to) {
                setError(
                  "请输入有效的本地日期和时间（24 小时制），该时间须在本地时区实际存在。",
                );
                return;
              }
              if (from >= to) {
                setError("结束时间须晚于开始时间。");
                return;
              }
              onChange({ from: from.toISOString(), to: to.toISOString() });
              close();
            }}
          >
            <div className="range-picker-body">
              <div className="datetime-range-grid">
                <DateTimeEditor
                  label="开始"
                  value={start}
                  onChange={setStart}
                  range={highlightedRange}
                />
                <DateTimeEditor
                  label="结束"
                  value={end}
                  onChange={setEnd}
                  range={highlightedRange}
                />
              </div>
              {error && (
                <div className="notice danger" role="alert">
                  {error}
                </div>
              )}
            </div>
            <div className="modal-footer">
              <button type="button" className="btn" onClick={close}>
                取消
              </button>
              <button type="submit" className="btn primary">
                应用时间范围
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}

function DateTimeEditor({
  label,
  value,
  onChange,
  range,
}: {
  label: string;
  value: { day: string; clock: string };
  onChange: (value: { day: string; clock: string }) => void;
  range?: { start: string; end: string };
}) {
  const initial = parseLocalDateTime(value.day, "12:00:00") || new Date();
  const [month, setMonth] = useState(
    new Date(initial.getFullYear(), initial.getMonth(), 1),
  );
  const grid = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const selected = parseLocalDateTime(value.day, "12:00:00");
    if (selected)
      setMonth(new Date(selected.getFullYear(), selected.getMonth(), 1));
  }, [value.day]);
  const start = new Date(month.getFullYear(), month.getMonth(), 1, 12);
  start.setDate(1 - ((start.getDay() + 6) % 7));
  const days = Array.from(
    { length: 42 },
    (_, i) =>
      new Date(start.getFullYear(), start.getMonth(), start.getDate() + i, 12),
  );
  const choose = (day: Date) => onChange({ ...value, day: localDate(day) });
  const focusDay =
    value.day.slice(0, 7) === localDate(month).slice(0, 7)
      ? value.day
      : localDate(new Date(month.getFullYear(), month.getMonth(), 1, 12));
  return (
    <section className="datetime-editor" aria-label={`${label}日期时间`}>
      <h3>{label}时间</h3>
      <div className="datetime-inputs">
        <label className="field">
          日期
          <input
            type="text"
            value={value.day}
            required
            placeholder="YYYY-MM-DD"
            autoComplete="off"
            aria-label={`${label}日期`}
            onChange={(e) => onChange({ ...value, day: e.target.value })}
          />
        </label>
        <label className="field">
          时间（24 小时）
          <input
            type="text"
            value={value.clock}
            required
            placeholder="HH:mm:ss"
            autoComplete="off"
            aria-label={`${label}时间`}
            onChange={(e) => onChange({ ...value, clock: e.target.value })}
          />
        </label>
      </div>
      <div className="calendar-header">
        <button
          type="button"
          className="icon-btn"
          aria-label={`${label}日历上个月`}
          onClick={() =>
            setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))
          }
        >
          <ChevronLeft size={16} />
        </button>
        <Select
          label={`${label}日历年份`}
          value={String(month.getFullYear())}
          onChange={(year) =>
            setMonth(new Date(Number(year), month.getMonth(), 1))
          }
          options={Array.from({ length: 61 }, (_, i) => ({
            value: String(month.getFullYear() - 50 + i),
            label: `${month.getFullYear() - 50 + i} 年`,
          }))}
        />
        <Select
          label={`${label}日历月份`}
          value={String(month.getMonth())}
          onChange={(m) =>
            setMonth(new Date(month.getFullYear(), Number(m), 1))
          }
          options={Array.from({ length: 12 }, (_, i) => ({
            value: String(i),
            label: `${i + 1} 月`,
          }))}
        />
        <button
          type="button"
          className="icon-btn"
          aria-label={`${label}日历下个月`}
          onClick={() =>
            setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))
          }
        >
          <ChevronRight size={16} />
        </button>
      </div>
      <div className="calendar-week" aria-hidden="true">
        {["一", "二", "三", "四", "五", "六", "日"].map((day) => (
          <span key={day}>{day}</span>
        ))}
      </div>
      <div
        className="calendar-grid"
        ref={grid}
        role="group"
        aria-label={`${label}日期日历`}
      >
        {days.map((day) => {
          const key = localDate(day),
            selected = key === range?.start || key === range?.end,
            inRange = !!range && key > range.start && key < range.end;
          return (
            <button
              type="button"
              key={key}
              data-date={key}
              tabIndex={key === focusDay ? 0 : -1}
              aria-label={`${label}日期 ${key}`}
              aria-pressed={key === value.day}
              className={`${selected || key === value.day ? "selected" : ""} ${inRange ? "in-range" : ""} ${key === range?.start ? "range-start" : ""} ${key === range?.end ? "range-end" : ""} ${day.getMonth() !== month.getMonth() ? "outside" : ""} ${key === localDate(new Date()) ? "today" : ""}`}
              onClick={() => choose(day)}
              onKeyDown={(e) => {
                const offsets: Record<string, number> = {
                  ArrowLeft: -1,
                  ArrowRight: 1,
                  ArrowUp: -7,
                  ArrowDown: 7,
                };
                let next: Date;
                if (e.key in offsets)
                  next = new Date(
                    day.getFullYear(),
                    day.getMonth(),
                    day.getDate() + offsets[e.key],
                    12,
                  );
                else if (e.key === "PageUp" || e.key === "PageDown") {
                  const m = day.getMonth() + (e.key === "PageUp" ? -1 : 1);
                  next = new Date(
                    day.getFullYear(),
                    m,
                    Math.min(
                      day.getDate(),
                      new Date(day.getFullYear(), m + 1, 0).getDate(),
                    ),
                    12,
                  );
                } else return;
                e.preventDefault();
                choose(next);
                requestAnimationFrame(() =>
                  grid.current
                    ?.querySelector<HTMLButtonElement>(
                      `[data-date="${localDate(next)}"]`,
                    )
                    ?.focus(),
                );
              }}
            >
              {day.getDate()}
            </button>
          );
        })}
      </div>
      <div className="calendar-help">
        <span>方向键选日期 · 可直接输入</span>
        <button
          type="button"
          className="text-btn"
          onClick={() => choose(new Date())}
        >
          今天
        </button>
      </div>
    </section>
  );
}
