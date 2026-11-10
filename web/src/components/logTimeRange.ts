export interface LogTimeRange {
  from: string;
  to: string;
  hours?: number;
}

export const logRangePresets = [
  { hours: 12, label: "12h" },
  { hours: 24, label: "1d" },
  { hours: 72, label: "3d" },
  { hours: 168, label: "7d" },
];

export function recentLogRange(hours = 12): LogTimeRange {
  const end = (Math.floor(Date.now() / 1000) + 1) * 1000;
  return {
    from: new Date(end - hours * 3600_000).toISOString(),
    to: new Date(end).toISOString(),
    hours,
  };
}

const pad = (n: number) => String(n).padStart(2, "0");
export function localDate(value: Date): string {
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}`;
}

export function localTime(value: Date): string {
  return `${pad(value.getHours())}:${pad(value.getMinutes())}:${pad(value.getSeconds())}`;
}

export function parseLocalDateTime(day: string, clock: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day) || !/^\d{2}:\d{2}(:\d{2})?$/.test(clock))
    return null;
  const value = new Date(`${day}T${clock}`);
  // 校验回读值，拒绝无效日期及夏令时跳跃中不存在的本地时间。
  if (
    !Number.isFinite(value.getTime()) ||
    localDate(value) !== day ||
    localTime(value) !== (clock.length === 5 ? `${clock}:00` : clock)
  )
    return null;
  return value;
}

export function rangeLabel(range: LogTimeRange): string {
  return [range.from, range.to]
    .map((iso) => {
      const value = new Date(iso);
      return `${localDate(value)} ${localTime(value)}`;
    })
    .join(" 至 ");
}
