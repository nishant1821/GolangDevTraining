"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { clearSession as clearStoredSession, getEmail, getToken, setSession as storeSession } from "@/lib/auth-storage";

interface AuthContextValue {
  email: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (token: string, email: string) => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

interface Session {
  email: string | null;
  isLoading: boolean;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session>({ email: null, isLoading: true });

  // Read from localStorage only after mount — avoids SSR/client hydration mismatch.
  // useSyncExternalStore was considered instead, but it would flash a
  // redirect-to-login for already-authenticated users before the real
  // snapshot resolves; a one-time mount read avoids that at the cost of an
  // extra initial render, which is fine for an auth gate.
  useEffect(() => {
    const token = getToken();
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSession({ email: token ? getEmail() : null, isLoading: false });
  }, []);

  const { email, isLoading } = session;

  function login(token: string, userEmail: string) {
    storeSession(token, userEmail);
    setSession({ email: userEmail, isLoading: false });
  }

  function logout() {
    clearStoredSession();
    setSession({ email: null, isLoading: false });
  }

  return (
    <AuthContext.Provider
      value={{ email, isAuthenticated: !!email, isLoading, login, logout }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
