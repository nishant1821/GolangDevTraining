"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  createMonitorSchema,
  type CreateMonitorFormValues,
  type CreateMonitorInput,
} from "@/lib/schemas";
import { useCreateMonitor } from "@/hooks/use-monitors";

export function CreateMonitorDialog() {
  const [open, setOpen] = useState(false);
  const { mutateAsync, isPending } = useCreateMonitor();

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<CreateMonitorFormValues, unknown, CreateMonitorInput>({
    resolver: zodResolver(createMonitorSchema),
    defaultValues: { interval_seconds: 60, timeout_seconds: 10 },
  });

  async function onSubmit(values: CreateMonitorInput) {
    try {
      await mutateAsync(values);
      reset();
      setOpen(false);
    } catch {
      // error toast already shown by the mutation's onError
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) reset();
      }}
    >
      <DialogTrigger asChild>
        <Button>
          <Plus className="size-4" />
          New monitor
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>New monitor</DialogTitle>
          <DialogDescription>
            Pulse will ping this URL on the interval you set and alert you when it goes down.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="name">Name</Label>
            <Input id="name" placeholder="Production API" {...register("name")} />
            {errors.name && <p className="text-sm text-destructive">{errors.name.message}</p>}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="url">URL</Label>
            <Input id="url" placeholder="https://example.com/health" {...register("url")} />
            {errors.url && <p className="text-sm text-destructive">{errors.url.message}</p>}
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="interval_seconds">Check interval (s)</Label>
              <Input id="interval_seconds" type="number" {...register("interval_seconds")} />
              {errors.interval_seconds && (
                <p className="text-sm text-destructive">{errors.interval_seconds.message}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="timeout_seconds">Timeout (s)</Label>
              <Input id="timeout_seconds" type="number" {...register("timeout_seconds")} />
              {errors.timeout_seconds && (
                <p className="text-sm text-destructive">{errors.timeout_seconds.message}</p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button type="submit" disabled={isPending}>
              {isPending ? "Creating…" : "Create monitor"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
