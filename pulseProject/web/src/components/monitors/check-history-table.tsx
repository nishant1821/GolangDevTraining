"use client";

import { format } from "date-fns";
import { CheckCircle2, XCircle } from "lucide-react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Check } from "@/types/pulse";

export function CheckHistoryTable({ checks }: { checks: Check[] }) {
  if (checks.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        No checks recorded yet — the first probe runs on the next scheduler tick.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Checked at</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>HTTP code</TableHead>
          <TableHead>Latency</TableHead>
          <TableHead>Error</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {checks.map((c) => (
          <TableRow key={c.id}>
            <TableCell className="text-muted-foreground">
              {format(new Date(c.checked_at), "PPp")}
            </TableCell>
            <TableCell>
              {c.up ? (
                <CheckCircle2 className="size-4 text-emerald-500" />
              ) : (
                <XCircle className="size-4 text-red-500" />
              )}
            </TableCell>
            <TableCell>{c.status_code || "—"}</TableCell>
            <TableCell>{c.response_time_ms}ms</TableCell>
            <TableCell className="max-w-[240px] truncate text-muted-foreground">
              {c.error || "—"}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
