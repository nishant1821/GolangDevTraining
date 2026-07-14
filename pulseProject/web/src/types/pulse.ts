// Mirrors internal/domain/models.go on the Go side.
// Field names match the `json:"..."` tags in the Go structs exactly.

export type MonitorStatus = "unknown" | "up" | "down";

export interface Monitor {
  id: number;
  created_at: string;
  updated_at: string;
  user_id: number;
  url: string;
  name: string;
  interval_seconds: number;
  timeout_seconds: number;
  active: boolean;
  status: MonitorStatus;
  next_check_at: string;
}

export interface Check {
  id: number;
  checked_at: string;
  monitor_id: number;
  status_code: number;
  response_time_ms: number;
  up: boolean;
  error?: string;
}

export interface Incident {
  id: number;
  created_at: string;
  monitor_id: number;
  started_at: string;
  resolved_at?: string;
}

export interface User {
  id: number;
  created_at: string;
  updated_at: string;
  email: string;
}
