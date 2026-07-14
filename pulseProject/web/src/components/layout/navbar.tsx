"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Activity, LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { useAuth } from "@/context/auth-context";

export function Navbar() {
  const { email, isAuthenticated, logout } = useAuth();
  const router = useRouter();

  function handleLogout() {
    logout();
    router.push("/login");
  }

  return (
    <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
        <Link href={isAuthenticated ? "/dashboard" : "/"} className="flex items-center gap-2 font-semibold">
          <Activity className="size-5 text-primary" />
          Pulse
        </Link>

        <div className="flex items-center gap-2">
          <ThemeToggle />
          {isAuthenticated && (
            <>
              <Avatar className="size-8">
                <AvatarFallback className="text-xs">
                  {email?.slice(0, 2).toUpperCase()}
                </AvatarFallback>
              </Avatar>
              <Button variant="ghost" size="icon" aria-label="Log out" onClick={handleLogout}>
                <LogOut className="size-4" />
              </Button>
            </>
          )}
        </div>
      </div>
    </header>
  );
}
