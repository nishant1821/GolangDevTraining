"use client";

import { useState } from "react";
import Link from "next/link";
import { formatDistanceToNow } from "date-fns";
import { MoreHorizontal, Pause, Play, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusBadge } from "@/components/monitors/status-badge";
import { useDeleteMonitor, usePauseMonitor, useResumeMonitor } from "@/hooks/use-monitors";
import type { Monitor } from "@/types/pulse";

// New monitors start with next_check_at at Go's zero value (year 1) until the
// scheduler's first tick sets a real timestamp — show "due now" instead of a
// nonsensical "in -2025 years".
function formatNextCheck(nextCheckAt: string) {
  const date = new Date(nextCheckAt);
  if (date.getFullYear() < 1970) return "due now";
  return formatDistanceToNow(date, { addSuffix: true });
}

export function MonitorTable({ monitors }: { monitors: Monitor[] }) {
  const [pendingDelete, setPendingDelete] = useState<Monitor | null>(null);
  const pause = usePauseMonitor();
  const resume = useResumeMonitor();
  const del = useDeleteMonitor();

  return (
    <>
      <div className="rounded-xl border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>URL</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Interval</TableHead>
              <TableHead>Next check</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {monitors.map((m) => (
              <TableRow key={m.id}>
                <TableCell className="font-medium">
                  <Link href={`/monitors/${m.id}`} className="hover:underline">
                    {m.name}
                  </Link>
                  {!m.active && (
                    <span className="ml-2 text-xs text-muted-foreground">(paused)</span>
                  )}
                </TableCell>
                <TableCell className="max-w-[280px] truncate text-muted-foreground">
                  {m.url}
                </TableCell>
                <TableCell>
                  <StatusBadge status={m.active ? m.status : "unknown"} />
                </TableCell>
                <TableCell className="text-muted-foreground">{m.interval_seconds}s</TableCell>
                <TableCell className="text-muted-foreground">
                  {m.active ? formatNextCheck(m.next_check_at) : "—"}
                </TableCell>
                <TableCell>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-sm" aria-label="Monitor actions">
                        <MoreHorizontal className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {m.active ? (
                        <DropdownMenuItem onClick={() => pause.mutate(m.id)}>
                          <Pause className="size-4" />
                          Pause
                        </DropdownMenuItem>
                      ) : (
                        <DropdownMenuItem onClick={() => resume.mutate(m.id)}>
                          <Play className="size-4" />
                          Resume
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={() => setPendingDelete(m)}
                      >
                        <Trash2 className="size-4" />
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <AlertDialog open={!!pendingDelete} onOpenChange={(open) => !open && setPendingDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete monitor?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently stop monitoring &ldquo;{pendingDelete?.name}&rdquo;. This
              action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pendingDelete) del.mutate(pendingDelete.id);
                setPendingDelete(null);
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
