export interface Account {
  resourceType: string;
  id: string;
  name: string;
  provider: string;
  region: string;
  maskedKey: string;
  status: string;
  testedAt: string;
  targetCount: number;
}
export interface Port {
  protocol: string;
  value: string;
}
export interface FixedIP {
  cidr: string;
  name?: string;
}
export interface Target {
  name: string;
  resourceType: string;
  id: string;
  accountId: string;
  region: string;
  groupId: string;
  groupName: string;
  ports: Port[];
  ipMode: string;
  fixedCidr: string;
  fixedIps?: FixedIP[];
  autoSync: boolean;
  note: string;
  currentCidr: string;
  currentCidrs?: string[];
  status: string;
  lastSync: string;
  error: string;
}
export interface Settings {
  detectOnManualSync: boolean;
  detectOnSchedule: boolean;
  timezone: string;
  enabled: boolean;
  interval: number;
  cron: string;
  sources: string[];
}
export interface State {
  accounts: Account[];
  targets: Target[];
  settings: Settings;
  ip: { ip: string; source: string; checkedAt: string; error: string };
  nextCheck: string;
  identity: string;
}
export interface Log {
  id: number;
  time: string;
  kind: string;
  trigger: string;
  object: string;
  detail: string;
  success: boolean;
  changes: string[];
  count?: number;
  firstId?: number;
  firstTime?: string;
}
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export async function api<T = unknown>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method,
    credentials: "same-origin",
    signal: AbortSignal.timeout(240_000),
    headers: {
      "X-GateSync-Request": "1",
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const result = await response.json();
  if (!response.ok)
    throw new ApiError(result.error || "请求失败，请稍后重试", response.status);
  return result;
}
export interface Region {
  id: string;
  name: string;
}
export interface NotificationChannel {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  events: string[];
  templates: Record<string, string>;
  hasWebhook: boolean;
  hasSignSecret: boolean;
}
export interface NotificationChannels {
  items: NotificationChannel[];
  events: Record<string, string>;
  templateNames: Record<string, string>;
  defaultTemplates: Record<string, string>;
  variables: string[];
}
export const kinds: Record<string, string> = {
  ip: "IP 检测",
  sync: "规则同步",
  account: "账号变更",
  target: "目标变更",
  settings: "计划设置",
  login: "登录",
  notification: "通知发送",
};
export function date(value?: string): string {
  if (!value) return "尚未执行";
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}
