import { z } from "zod";

export const loginSchema = z.object({
  email: z.string().min(1, "Email is required").email("Enter a valid email"),
  password: z.string().min(1, "Password is required"),
});
export type LoginInput = z.infer<typeof loginSchema>;

export const registerSchema = z
  .object({
    email: z.string().min(1, "Email is required").email("Enter a valid email"),
    password: z.string().min(8, "Password must be at least 8 characters"),
    confirmPassword: z.string().min(1, "Confirm your password"),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: "Passwords do not match",
    path: ["confirmPassword"],
  });
export type RegisterInput = z.infer<typeof registerSchema>;

// Mirrors the `validate:"..."` tags on createReq in internal/handler/monitor_handler.go
export const createMonitorSchema = z.object({
  name: z.string().min(1, "Name is required").max(255),
  url: z.string().min(1, "URL is required").url("Enter a valid URL (include https://)"),
  interval_seconds: z.coerce
    .number()
    .int()
    .min(5, "Minimum interval is 5 seconds")
    .max(86400, "Maximum interval is 86400 seconds (24h)"),
  timeout_seconds: z.coerce
    .number()
    .int()
    .min(1, "Minimum timeout is 1 second")
    .max(60, "Maximum timeout is 60 seconds"),
});
// z.coerce fields have a different input type (unknown, from an <input>) than
// output type (number) — react-hook-form needs both to wire up the resolver.
export type CreateMonitorFormValues = z.input<typeof createMonitorSchema>;
export type CreateMonitorInput = z.output<typeof createMonitorSchema>;
