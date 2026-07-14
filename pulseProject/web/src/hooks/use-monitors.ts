import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ApiError,
  createMonitor,
  deleteMonitor,
  getCheckHistory,
  getMonitor,
  listMonitors,
  pauseMonitor,
  resumeMonitor,
  type CreateMonitorInput,
} from "@/lib/api";

export const monitorKeys = {
  all: ["monitors"] as const,
  list: () => [...monitorKeys.all, "list"] as const,
  detail: (id: number) => [...monitorKeys.all, "detail", id] as const,
  checks: (id: number) => [...monitorKeys.all, "checks", id] as const,
};

// Polling interval — short enough to feel "live" without hammering the API.
// The Go scheduler itself defaults to a 30s tick (SCHEDULE_INTERVAL_SECONDS).
const POLL_MS = 15_000;

export function useMonitors() {
  return useQuery({
    queryKey: monitorKeys.list(),
    queryFn: () => listMonitors(),
    refetchInterval: POLL_MS,
  });
}

export function useMonitor(id: number) {
  return useQuery({
    queryKey: monitorKeys.detail(id),
    queryFn: () => getMonitor(id),
    refetchInterval: POLL_MS,
    enabled: Number.isFinite(id),
  });
}

export function useCheckHistory(id: number, page = 1, pageSize = 50) {
  return useQuery({
    queryKey: [...monitorKeys.checks(id), page, pageSize],
    queryFn: () => getCheckHistory(id, page, pageSize),
    refetchInterval: POLL_MS,
    enabled: Number.isFinite(id),
  });
}

function errMsg(err: unknown) {
  return err instanceof ApiError ? err.message : "Something went wrong";
}

export function useCreateMonitor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateMonitorInput) => createMonitor(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: monitorKeys.list() });
      toast.success("Monitor created");
    },
    onError: (err) => toast.error(errMsg(err)),
  });
}

export function usePauseMonitor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => pauseMonitor(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: monitorKeys.list() });
      qc.invalidateQueries({ queryKey: monitorKeys.detail(id) });
      toast.success("Monitor paused");
    },
    onError: (err) => toast.error(errMsg(err)),
  });
}

export function useResumeMonitor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => resumeMonitor(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: monitorKeys.list() });
      qc.invalidateQueries({ queryKey: monitorKeys.detail(id) });
      toast.success("Monitor resumed");
    },
    onError: (err) => toast.error(errMsg(err)),
  });
}

export function useDeleteMonitor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteMonitor(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: monitorKeys.list() });
      toast.success("Monitor deleted");
    },
    onError: (err) => toast.error(errMsg(err)),
  });
}
