"use client";

import Link from "next/link";
import { Cable, KeyRound, PlugZap, ScrollText, Shield, Wand2 } from "lucide-react";
import { NavItem } from "@/components/shell/nav-item";
import { UserFooter } from "@/components/shell/user-footer";
import { Separator } from "@/components/ui/separator";

const navItems = [
  { href: "/dashboard", label: "Connections", icon: Cable, exact: true },
  { href: "/dashboard/sources", label: "Sources", icon: PlugZap },
  { href: "/dashboard/guardrails", label: "Guardrails", icon: Shield },
  { href: "/dashboard/transformations", label: "Transformations", icon: Wand2 },
  { href: "/dashboard/audit-logs", label: "Audit Logs", icon: ScrollText },
  { href: "/dashboard/api-keys", label: "API Keys", icon: KeyRound },
];

export function SidebarNav({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <>
      <Link
        href="/dashboard"
        onClick={onNavigate}
        className="flex items-center gap-2 px-3 py-4"
      >
        <Shield className="h-6 w-6 text-primary" />
        <span className="text-lg font-bold tracking-tight">Sheeld</span>
      </Link>
      <Separator className="mb-3" />
      <nav className="flex flex-1 flex-col gap-1 px-2">
        {navItems.map((item) => (
          <NavItem key={item.href} {...item} onNavigate={onNavigate} />
        ))}
      </nav>
    </>
  );
}

export function Sidebar() {
  return (
    <aside className="hidden w-60 shrink-0 flex-col border-r bg-sidebar md:flex">
      <SidebarNav />
      <Separator className="mt-3" />
      <div className="p-2">
        <UserFooter />
      </div>
    </aside>
  );
}
