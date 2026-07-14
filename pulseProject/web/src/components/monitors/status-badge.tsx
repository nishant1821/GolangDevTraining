import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { MonitorStatus } from "@/types/pulse";

const STYLES: Record<MonitorStatus, string> = {
  up: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/20",
  down: "bg-red-500/15 text-red-600 dark:text-red-400 border-red-500/20",
  unknown: "bg-muted text-muted-foreground border-transparent",
};

const LABELS: Record<MonitorStatus, string> = {
  up: "Up",
  down: "Down",
  unknown: "Unknown",
};

export function StatusBadge({ status }: { status: MonitorStatus }) {
  return (
    <Badge variant="outline" className={cn("gap-1.5 font-medium", STYLES[status])}>
      <span
        className={cn("size-1.5 rounded-full", {
          "bg-emerald-500": status === "up",
          "bg-red-500": status === "down",
          "bg-muted-foreground": status === "unknown",
        })}
      />
      {LABELS[status]}
    </Badge>
  );
}
