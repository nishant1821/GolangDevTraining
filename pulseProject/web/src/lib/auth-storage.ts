// Client-side JWT storage. The Go API issues a signed JWT with no refresh
// token, so we keep it simple: one token in localStorage, read/written only
// from client components/hooks (never during SSR).

const TOKEN_KEY = "pulse_token";
const EMAIL_KEY = "pulse_email";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(TOKEN_KEY);
}

export function setSession(token: string, email: string) {
  window.localStorage.setItem(TOKEN_KEY, token);
  window.localStorage.setItem(EMAIL_KEY, email);
}

export function getEmail(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(EMAIL_KEY);
}

export function clearSession() {
  window.localStorage.removeItem(TOKEN_KEY);
  window.localStorage.removeItem(EMAIL_KEY);
}
