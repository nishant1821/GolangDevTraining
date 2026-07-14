import { notFound } from "next/navigation";
import { MonitorDetail } from "./monitor-detail";

export default async function MonitorPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const numericId = Number(id);
  if (!Number.isFinite(numericId)) notFound();

  return <MonitorDetail id={numericId} />;
}
