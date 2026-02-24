"use client";

import { useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

const navItems = [
  { href: "/dashboard", label: "Sources" },
  { href: "/dashboard/api-keys", label: "API Keys" },
  { href: "/dashboard/audit-logs", label: "Audit Logs" },
];

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const { token, user, loading, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (!loading && !token) {
      router.replace("/login");
    }
  }, [token, loading, router]);

  if (loading || !token) return null;

  return (
    <div className="flex min-h-screen">
      <aside className="w-64 border-r bg-muted/40 p-4 flex flex-col">
        <div className="mb-6">
          <h1 className="text-xl font-bold">Sheeld</h1>
          <p className="text-sm text-muted-foreground truncate">{user?.email}</p>
        </div>
        <Separator className="mb-4" />
        <nav className="flex flex-col gap-1 flex-1">
          {navItems.map((item) => {
            const isActive =
              item.href === "/dashboard"
                ? pathname === "/dashboard" || pathname.startsWith("/dashboard/sources")
                : pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-primary text-primary-foreground"
                    : "hover:bg-muted"
                }`}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>
        <Separator className="my-4" />
        <Button variant="outline" size="sm" onClick={logout}>
          Sign out
        </Button>
      </aside>
      <main className="flex-1 p-6">{children}</main>
    </div>
  );
}
