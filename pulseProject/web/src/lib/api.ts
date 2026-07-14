import { clearSession, getToken } from "@/lib/auth-storage";
import type { Check, Monitor } from "@/types/pulse";

// Mirrors internal/handler/respond.go's envelope shape.
interface Envelope<T> {
  success: boolean;
  data?: T;
  error?: string;
  message?: string;
  request_id?: string;
}

export class ApiError extends Error {
  status: number;
  code?: string;
  requestId?: string;

  constructor(status: number, message: string, code?: string, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers = new Headers(options.headers);
  headers.set("Content-Type", "application/json");
  if (token) headers.set("Authorization", `Bearer ${token}`);

  const res = await fetch(path, { ...options, headers });

  // 204 No Content — pause/resume/delete return no body.
  if (res.status === 204) return undefined as T;

  const body = (await res.json().catch(() => ({}))) as Envelope<T>;

  if (!res.ok || body.success === false) {
    if (res.status === 401) {
      clearSession();
      if (typeof window !== "undefined") {
        window.location.href = "/login";
      }
    }
    throw new ApiError(res.status, body.message ?? res.statusText, body.error, body.request_id);
  }

  return body.data as T;
}

// ── Auth ─────────────────────────────────────────────────────────────────

export function registerUser(email: string, password: string) {
  return request<{ id: number; email: string }>("/api/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function loginUser(email: string, password: string) {
  return request<{ token: string }>("/api/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

// ── Monitors ─────────────────────────────────────────────────────────────

export function listMonitors(page = 1, pageSize = 20) {
  return request<Monitor[]>(`/api/monitors?page=${page}&page_size=${pageSize}`);
}

export function getMonitor(id: number) {
  return request<Monitor>(`/api/monitors/${id}`);
}

export interface CreateMonitorInput {
  url: string;
  name: string;
  interval_seconds: number;
  timeout_seconds: number;
}

export function createMonitor(input: CreateMonitorInput) {
  return request<Monitor>("/api/monitors", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function pauseMonitor(id: number) {
  return request<void>(`/api/monitors/${id}/pause`, { method: "PATCH" });
}

export function resumeMonitor(id: number) {
  return request<void>(`/api/monitors/${id}/resume`, { method: "PATCH" });
}

export function deleteMonitor(id: number) {
  return request<void>(`/api/monitors/${id}`, { method: "DELETE" });
}

export function getCheckHistory(id: number, page = 1, pageSize = 50) {
  return request<Check[]>(`/api/monitors/${id}/checks?page=${page}&page_size=${pageSize}`);
}
