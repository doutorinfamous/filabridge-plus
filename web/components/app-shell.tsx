"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Database,
  History,
  LayoutDashboard,
  Link2,
  Nfc,
  Settings,
} from "lucide-react";

import { cn } from "@/lib/utils";

const navItems = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard },
  { href: "/history", label: "History", icon: History },
  { href: "/nfc", label: "NFC & QR", icon: Nfc },
  { href: "/settings", label: "Settings", icon: Settings },
];

const tempNavItems = [
  {
    href: "/temp/database",
    label: "Database",
    icon: Database,
    className: "text-amber-600/90 hover:text-amber-500",
  },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  if (pathname.startsWith("/nfc/scan")) {
    return <>{children}</>;
  }

  return (
    <div className="flex min-h-svh">
      {/* Sidebar */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col border-r border-sidebar-border bg-sidebar md:flex">
        <div className="flex h-16 items-center gap-2.5 border-b border-sidebar-border px-5">
          <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Link2 className="size-4" />
          </div>
          <div>
            <p className="text-sm font-semibold leading-tight">FilaBridge</p>
            <p className="text-[11px] leading-tight text-muted-foreground">
              Filament inventory bridge
            </p>
          </div>
        </div>
        <nav className="flex flex-1 flex-col gap-1 p-3">
          {navItems.map((item) => {
            const active =
              item.href === "/"
                ? pathname === "/"
                : pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                  active
                    ? "bg-sidebar-accent text-sidebar-accent-foreground"
                    : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground"
                )}
              >
                <item.icon className="size-4" />
                {item.label}
              </Link>
            );
          })}
          <div className="my-2 border-t border-sidebar-border" />
          {tempNavItems.map((item) => {
            const active = pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                  active
                    ? "bg-sidebar-accent text-sidebar-accent-foreground"
                    : "hover:bg-sidebar-accent/60",
                  item.className
                )}
              >
                <item.icon className="size-4" />
                {item.label}
              </Link>
            );
          })}
        </nav>
        <div className="border-t border-sidebar-border p-4 text-[11px] text-muted-foreground">
          <Link
            href="https://github.com/doutorinfamous"
            target="_blank"
            rel="noopener noreferrer"
            className="transition-colors hover:text-foreground"
          >
           created and maintained by Papai Nerd
          </Link>
        </div>
      </aside>

      {/* Mobile top bar */}
      <div className="fixed inset-x-0 top-0 z-30 flex h-14 items-center justify-between border-b border-border bg-background/80 px-4 backdrop-blur md:hidden">
        <div className="flex items-center gap-2">
          <div className="flex size-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Link2 className="size-3.5" />
          </div>
          <span className="text-sm font-semibold">FilaBridge</span>
        </div>
        <nav className="flex items-center gap-1">
          {navItems.map((item) => {
            const active =
              item.href === "/"
                ? pathname === "/"
                : pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "rounded-md p-2",
                  active
                    ? "bg-accent text-accent-foreground"
                    : "text-muted-foreground"
                )}
                aria-label={item.label}
              >
                <item.icon className="size-4.5" />
              </Link>
            );
          })}
          {tempNavItems.map((item) => {
            const active = pathname.startsWith(item.href);
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "rounded-md p-2",
                  active ? "bg-accent text-accent-foreground" : item.className
                )}
                aria-label={item.label}
              >
                <item.icon className="size-4.5" />
              </Link>
            );
          })}
        </nav>
      </div>

      <main className="min-w-0 flex-1 overflow-x-hidden pt-14 md:ml-60 md:pt-0">
        <div className="mx-auto w-full min-w-0 max-w-6xl px-4 py-6 md:px-8 md:py-8">
          {children}
        </div>
      </main>
    </div>
  );
}
