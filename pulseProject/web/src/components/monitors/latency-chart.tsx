"use client";

import { format } from "date-fns";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { Check } from "@/types/pulse";

export function LatencyChart({ checks }: { checks: Check[] }) {
  // Checks come back newest-first from the API — reverse for a left-to-right timeline.
  const data = [...checks]
    .reverse()
    .map((c) => ({
      time: c.checked_at,
      latency: c.response_time_ms,
      up: c.up,
    }));

  return (
    <ResponsiveContainer width="100%" height={240}>
      <AreaChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id="latencyFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--chart-2)" stopOpacity={0.35} />
            <stop offset="100%" stopColor="var(--chart-2)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" className="stroke-border" vertical={false} />
        <XAxis
          dataKey="time"
          tickFormatter={(v) => format(new Date(v), "HH:mm")}
          className="text-xs fill-muted-foreground"
          tickLine={false}
          axisLine={false}
          minTickGap={40}
        />
        <YAxis
          className="text-xs fill-muted-foreground"
          tickLine={false}
          axisLine={false}
          width={48}
          tickFormatter={(v) => `${v}ms`}
        />
        <Tooltip
          contentStyle={{
            background: "var(--popover)",
            border: "1px solid var(--border)",
            borderRadius: "var(--radius-lg)",
            color: "var(--popover-foreground)",
            fontSize: 12,
          }}
          labelFormatter={(v) => format(new Date(v), "PPpp")}
          formatter={(value) => [`${value}ms`, "Latency"]}
        />
        <Area
          type="monotone"
          dataKey="latency"
          stroke="var(--chart-2)"
          strokeWidth={2}
          fill="url(#latencyFill)"
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}
