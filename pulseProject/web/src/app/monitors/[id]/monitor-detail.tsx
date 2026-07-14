"use client";

import Link from "next/link";
import { ArrowLeft, ExternalLink } from "lucide-react";
import { AuthGuard } from "@/components/auth/auth-guard";
import { StatusBadge } from "@/components/monitors/status-badge";
import { LatencyChart } from "@/components/monitors/latency-chart";
import { CheckHistoryTable } from "@/components/monitors/check-history-table";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { usePauseMonitor, useResumeMonitor, useMonitor, useCheckHistory } from "@/hooks/use-monitors";

function uptimePercent(checks: { up: boolean }[]) {
  if (checks.length === 0) return null;
  const upCount = checks.filter((c) => c.up).length;
  return ((upCount / checks.length) * 100).toFixed(1);
}

function avgLatency(checks: { response_time_ms: number; up: boolean }[]) {
  const ok = checks.filter((c) => c.up);
  if (ok.length === 0) return null;
  return Math.round(ok.reduce((sum, c) => sum + c.response_time_ms, 0) / ok.length);
}

function MonitorDetailContent({ id }: { id: number }) {
  const { data: monitor, isLoading: monitorLoading } = useMonitor(id);
  const { data: checks, isLoading: checksLoading } = useCheckHistory(id, 1, 100);
  const pause = usePauseMonitor();
  const resume = useResumeMonitor();

  if (monitorLoading || !monitor) {
    return (
      <div className="mx-auto w-full max-w-6xl space-y-4 p-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  const uptime = checks ? uptimePercent(checks) : null;
  const latency = checks ? avgLatency(checks) : null;

  return (
    <div className="mx-auto w-full max-w-6xl flex-1 space-y-6 p-6">
      <Link
        href="/dashboard"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" />
        Back to monitors
      </Link>

      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">{monitor.name}</h1>
            <StatusBadge status={monitor.active ? monitor.status : "unknown"} />
          </div>
          <a
            href={monitor.url}
            target="_blank"
            rel="noopener noreferrer"
            className="mt-1 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          >
            {monitor.url}
            <ExternalLink className="size-3" />
          </a>
        </div>
        <div className="flex gap-2">
          {monitor.active ? (
            <Button variant="outline" onClick={() => pause.mutate(monitor.id)}>
              Pause
            </Button>
          ) : (
            <Button variant="outline" onClick={() => resume.mutate(monitor.id)}>
              Resume
            </Button>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-normal text-muted-foreground">
              Uptime (last 100 checks)
            </CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-semibold">
            {uptime !== null ? `${uptime}%` : "—"}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-normal text-muted-foreground">
              Avg. latency
            </CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-semibold">
            {latency !== null ? `${latency}ms` : "—"}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-normal text-muted-foreground">
              Check interval
            </CardTitle>
          </CardHeader>
          <CardContent className="text-2xl font-semibold">
            {monitor.interval_seconds}s
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Latency over time</CardTitle>
        </CardHeader>
        <CardContent>
          {checksLoading && <Skeleton className="h-60 w-full" />}
          {!checksLoading && checks && <LatencyChart checks={checks} />}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Check history</CardTitle>
        </CardHeader>
        <CardContent>
          {checksLoading && <Skeleton className="h-40 w-full" />}
          {!checksLoading && checks && <CheckHistoryTable checks={checks} />}
        </CardContent>
      </Card>
    </div>
  );
}

export function MonitorDetail({ id }: { id: number }) {
  return (
    <AuthGuard>
      <MonitorDetailContent id={id} />
    </AuthGuard>
  );
}
