// 管理端 API 访问：同源 fetch，写操作带 X-Admin-Request 头，401 统一抛出。

export class UnauthorizedError extends Error {
  constructor() {
    super("未登录或登录已过期");
  }
}

export class ApiError extends Error {
  code: number;
  constructor(code: number, msg: string) {
    super(msg);
    this.code = code;
  }
}

// adminPath 从当前页面路径推导密钥段（SPA 挂在 /<secret>/ 下）。
export function adminPath(): string {
  const seg = window.location.pathname.split("/").filter(Boolean)[0];
  return seg ? "/" + seg : "";
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const headers: Record<string, string> = {
    ...((init?.headers as Record<string, string>) ?? {}),
  };
  if (method !== "GET") {
    headers["X-Admin-Request"] = "1";
  }
  if (init?.body) {
    headers["Content-Type"] = "application/json";
  }
  const res = await fetch(adminPath() + "/api" + path, {
    credentials: "same-origin",
    ...init,
    headers,
  });
  if (res.status === 401) {
    throw new UnauthorizedError();
  }
  // 204 或空响应体（logout / revoke）没有 JSON 可解。
  if (res.status === 204) {
    return undefined as T;
  }
  const text = await res.text();
  if (!text) {
    return undefined as T;
  }
  const body = JSON.parse(text);
  if (body.code !== 0) {
    throw new ApiError(body.code as number, body.msg as string);
  }
  return body.data as T;
}

export interface Me {
  username: string;
  perm: string;
  exp: number;
  sat: number;
}

export interface FlagSet {
  search: boolean;
  detail: boolean;
  catalog: boolean;
  content: boolean;
}

export interface IpFilter {
  mode: string;
  scope: string;
  rules: string[];
}

export interface AppFrontends {
  current: string;
  previous: string;
  versions: { version: string; size: number; files: number; created_at: number }[];
}

export function login(username: string, password: string): Promise<{ username: string }> {
  return apiFetch("/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
}

export function logout(): Promise<void> {
  return apiFetch("/auth/logout", { method: "POST" });
}

export function me(): Promise<Me> {
  return apiFetch<Me>("/auth/me");
}

export function revoke(): Promise<void> {
  return apiFetch("/auth/revoke", { method: "POST" });
}

export function getFlags(): Promise<FlagSet> {
  return apiFetch<FlagSet>("/flags");
}

export function getIpFilter(): Promise<IpFilter> {
  return apiFetch<IpFilter>("/ip-filter");
}

export function getFrontends(): Promise<{ reader: AppFrontends; admin: AppFrontends }> {
  return apiFetch("/frontends");
}

export function setFlag(name: string, enabled: boolean): Promise<void> {
  return apiFetch("/flags/" + name, {
    method: "PUT",
    body: JSON.stringify({ enabled }),
  });
}

export interface RateLimitRule {
  mode: string;
  per_minute: number;
  burst: number;
}

export type RateLimits = {
  search: RateLimitRule;
  detail: RateLimitRule;
  catalog: RateLimitRule;
  content: RateLimitRule;
};

export function getRateLimits(): Promise<RateLimits> {
  return apiFetch<RateLimits>("/rate-limits");
}

export function putRateLimits(rl: RateLimits): Promise<RateLimits> {
  return apiFetch<RateLimits>("/rate-limits", {
    method: "PUT",
    body: JSON.stringify(rl),
  });
}

export function putIpFilter(f: IpFilter): Promise<IpFilter> {
  return apiFetch<IpFilter>("/ip-filter", {
    method: "PUT",
    body: JSON.stringify(f),
  });
}

export interface LogConfig {
  level: string;
  max_size_mb: number;
  max_backups: number;
  max_age_days: number;
  compress: boolean;
}

export function getLogConfig(): Promise<LogConfig> {
  return apiFetch<LogConfig>("/config/log");
}

export function putLogConfig(c: LogConfig): Promise<LogConfig> {
  return apiFetch<LogConfig>("/config/log", {
    method: "PUT",
    body: JSON.stringify(c),
  });
}

export interface UpstreamConfig {
  timeout_seconds: number;
  max_concurrency: number;
}

export function getUpstreamConfig(): Promise<UpstreamConfig> {
  return apiFetch<UpstreamConfig>("/config/upstream");
}

export function putUpstreamConfig(c: UpstreamConfig): Promise<UpstreamConfig> {
  return apiFetch<UpstreamConfig>("/config/upstream", {
    method: "PUT",
    body: JSON.stringify(c),
  });
}

export function changePassword(oldPassword: string, newPassword: string): Promise<void> {
  return apiFetch("/account/password", {
    method: "POST",
    body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
  });
}
