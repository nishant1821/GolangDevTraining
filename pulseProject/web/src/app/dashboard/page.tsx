"use client";

import { AuthGuard } from "@/components/auth/auth-guard";
import { CreateMonitorDialog } from "@/components/monitors/create-monitor-dialog";
import { MonitorTable } from "@/components/monitors/monitor-table";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useMonitors } from "@/hooks/use-monitors";
import { Activity } from "lucide-react";

function DashboardContent() {
  const { data: monitors, isLoading, isError } = useMonitors();

  return (
    <div className="mx-auto w-full max-w-6xl flex-1 space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Monitors</h1>
          <p className="text-sm text-muted-foreground">
            {monitors?.length ?? 0} monitor{monitors?.length === 1 ? "" : "s"}
          </p>
        </div>
        <CreateMonitorDialog />
      </div>

      {isLoading && (
        <div className="space-y-2">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      )}

      {isError && (
        <Card>
          <CardContent className="py-10 text-center text-muted-foreground">
            Couldn&apos;t load monitors. Is the Pulse API running?
          </CardContent>
        </Card>
      )}

      {!isLoading && !isError && monitors?.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
            <Activity className="size-10 text-muted-foreground" />
            <div>
              <p className="font-medium">No monitors yet</p>
              <p className="text-sm text-muted-foreground">
                Create your first monitor to start tracking uptime.
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      {!isLoading && !isError && monitors && monitors.length > 0 && (
        <MonitorTable monitors={monitors} />
      )}
    </div>
  );
}

export default function DashboardPage() {
  return (
    <AuthGuard>
      <DashboardContent />
    </AuthGuard>
  );
}
