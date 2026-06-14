"use client";

import * as React from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  AutoAssignSettings,
  PollingSettings,
  TimeoutSettings,
} from "@/components/settings/advanced-settings";
import {
  HomeAssistantSettings,
  SpoolmanSettings,
} from "@/components/settings/general-settings";
import { PrintersSettings } from "@/components/settings/printers-settings";
import { DatabaseBrowser } from "@/components/_temp/database-browser";

const VALID_TABS = [
  "spoolman",
  "home-assistant",
  "printers",
  "advanced",
  "database",
] as const;

function normalizeTab(value: string | null): string {
  if (value === "general") return "spoolman";
  if (value && (VALID_TABS as readonly string[]).includes(value)) return value;
  return "spoolman";
}

function SettingsContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const tab = normalizeTab(searchParams.get("tab"));

  const setTab = (value: string) => {
    router.replace(`${pathname}?tab=${value}`, { scroll: false });
  };

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Spoolman, Home Assistant, printers, and FilaBridge+ behavior
        </p>
      </header>

      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList>
          <TabsTrigger value="spoolman">Spoolman</TabsTrigger>
          <TabsTrigger value="home-assistant">Home Assistant</TabsTrigger>
          <TabsTrigger value="printers">Printers</TabsTrigger>
          <TabsTrigger value="advanced">Advanced</TabsTrigger>
          <TabsTrigger value="database">Database</TabsTrigger>
        </TabsList>

        <TabsContent value="spoolman" className="space-y-4">
          <SpoolmanSettings />
        </TabsContent>

        <TabsContent value="home-assistant" className="space-y-4">
          <HomeAssistantSettings />
        </TabsContent>

        <TabsContent value="printers">
          <PrintersSettings />
        </TabsContent>

        <TabsContent value="advanced" className="space-y-4">
          <TimeoutSettings />
          <PollingSettings />
          <AutoAssignSettings />
        </TabsContent>

        <TabsContent value="database" className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Read-only inspection of the local SQLite database (filabridge.db),
            with schema and data updated in real time
          </p>
          <DatabaseBrowser />
        </TabsContent>
      </Tabs>
    </div>
  );
}

export default function SettingsPage() {
  return (
    <React.Suspense>
      <SettingsContent />
    </React.Suspense>
  );
}
