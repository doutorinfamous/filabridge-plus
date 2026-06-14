"use client";

import * as React from "react";
import Link from "next/link";
import { AlertTriangle, Link2, Nfc } from "lucide-react";
import { useSearchParams } from "next/navigation";

import { ScanConnectionBeam } from "@/components/nfc/scan-connection-beam";
import { ScanHeroLocation } from "@/components/nfc/scan-hero-location";
import { ScanHeroSpool } from "@/components/nfc/scan-hero-spool";
import { ScanLayout } from "@/components/nfc/scan-layout";
import {
  ScanSuccess,
  type ScanSuccessData,
} from "@/components/nfc/scan-success";
import { ScanSpoolPicker } from "@/components/nfc/scan-spool-picker";
import { ScanWaitingCard } from "@/components/nfc/scan-waiting-card";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import type { NfcSelectSpoolSuccess, NfcSessionStatus } from "@/lib/types";

function parseSuccessParams(params: URLSearchParams): ScanSuccessData | null {
  if (params.get("success") !== "1") return null;
  const spoolId = Number(params.get("spool_id"));
  const location = params.get("location");
  if (!spoolId || !location) return null;

  const locationType = params.get("location_type") as
    | ScanSuccessData["locationType"]
    | null;

  return {
    spoolId,
    spoolName: params.get("spool_name") ?? undefined,
    spoolMaterial: params.get("spool_material") ?? undefined,
    spoolBrand: params.get("spool_brand") ?? undefined,
    spoolColor: params.get("spool_color") ?? undefined,
    spoolWeight: params.get("spool_weight")
      ? Number(params.get("spool_weight"))
      : undefined,
    location,
    locationType: locationType ?? undefined,
    printer: params.get("printer") ?? undefined,
    toolhead: params.get("toolhead") ?? undefined,
    slot: params.get("slot") ?? undefined,
  };
}

function accentFromSession(session: NfcSessionStatus): string | undefined {
  const hex = session.spool?.color_hex ?? session.pending_filament?.color_hex;
  if (!hex) return undefined;
  return hex.startsWith("#") ? hex : `#${hex}`;
}

function formatScanError(
  errorCode: string,
  searchParams: URLSearchParams
): string {
  if (errorCode === "no_spools_for_filament") {
    const filamentName = searchParams.get("filament_name");
    return filamentName
      ? `No spools available for ${filamentName}.`
      : "No spools available for this filament.";
  }
  return errorCode;
}

function selectSuccessToScanData(
  success: NfcSelectSpoolSuccess
): ScanSuccessData {
  return {
    spoolId: success.spool_id,
    spoolName: success.spool_name,
    spoolMaterial: success.spool_material,
    spoolBrand: success.spool_brand,
    spoolColor: success.spool_color,
    spoolWeight: success.spool_weight,
    location: success.location,
    locationType: success.location_type,
    printer: success.printer,
    toolhead: success.toolhead,
    slot: success.slot,
  };
}

function ScanEmptyState() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center px-6 py-12 text-center">
      <div className="mb-6 flex size-20 items-center justify-center rounded-2xl border border-white/10 bg-white/[0.04]">
        <Nfc className="size-10 text-white/40" strokeWidth={1.25} />
      </div>
      <h1 className="text-xl font-semibold">Ready to scan</h1>
      <p className="mt-2 max-w-xs text-sm text-white/50">
        Hold your phone near an NFC tag or scan a spool or location QR code to
        start pairing.
      </p>
      <Button asChild variant="outline" className="mt-8 min-h-11">
        <Link href="/">Back to dashboard</Link>
      </Button>
    </div>
  );
}

function ScanErrorState({ message }: { message: string }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center px-6 py-12 text-center">
      <div className="mb-6 flex size-20 items-center justify-center rounded-2xl border border-red-500/20 bg-red-500/10">
        <AlertTriangle className="size-10 text-red-400" strokeWidth={1.25} />
      </div>
      <h1 className="text-xl font-semibold">Scan error</h1>
      <p className="mt-2 max-w-sm text-sm text-white/60">{message}</p>
      <Button asChild className="mt-8 min-h-11">
        <Link href="/">Back to dashboard</Link>
      </Button>
    </div>
  );
}

function ScanPairingView({ session }: { session: NfcSessionStatus }) {
  const spoolFirst =
    session.has_spool && !session.has_location && !session.has_pending_filament;
  const locationFirst =
    session.has_location && !session.has_spool && !session.has_pending_filament;

  return (
    <div className="flex flex-1 flex-col px-5 py-8">
      <header className="mb-8 text-center">
        <p className="font-mono text-[10px] uppercase tracking-[0.3em] text-white/40">
          FilaBridge+ · NFC
        </p>
        <h1 className="mt-2 text-xl font-semibold tracking-tight">
          {spoolFirst
            ? "Spool paired — waiting"
            : locationFirst
              ? "Location paired — waiting"
              : "Pairing in progress"}
        </h1>
        <p className="mt-1 text-sm text-white/50">
          {spoolFirst
            ? "Scan the location tag next"
            : locationFirst
              ? "Scan the spool tag next"
              : "Complete both scans"}
        </p>
      </header>

      <div className="mx-auto w-full max-w-md flex-1 space-y-0">
        {spoolFirst && session.spool && (
          <>
            <ScanHeroSpool spool={session.spool} />
            <ScanConnectionBeam />
            <ScanWaitingCard kind="location" />
          </>
        )}

        {locationFirst && session.location && (
          <>
            <ScanHeroLocation location={session.location} />
            <ScanConnectionBeam />
            <ScanWaitingCard kind="spool" />
          </>
        )}

        {!spoolFirst && !locationFirst && <ScanEmptyState />}
      </div>

      <footer className="mt-auto pt-8 text-center">
        <div className="inline-flex items-center gap-2 text-white/30">
          <Link2 className="size-3.5" />
          <span className="font-mono text-[10px] uppercase tracking-widest">
            FilaBridge+
          </span>
        </div>
      </footer>
    </div>
  );
}

export default function NfcScanContent() {
  const searchParams = useSearchParams();
  const [session, setSession] = React.useState<NfcSessionStatus | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [selecting, setSelecting] = React.useState(false);
  const [pickedSuccess, setPickedSuccess] =
    React.useState<ScanSuccessData | null>(null);

  const sessionId = searchParams.get("session_id") ?? undefined;
  const errorCode = searchParams.get("error");
  const errorMessage = errorCode
    ? formatScanError(errorCode, searchParams)
    : null;
  const successData = React.useMemo(
    () => pickedSuccess ?? parseSuccessParams(searchParams),
    [pickedSuccess, searchParams]
  );

  React.useEffect(() => {
    if (errorMessage || successData) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    api
      .getNfcSessionStatus(sessionId)
      .then((status) => {
        if (!cancelled) setSession(status);
      })
      .catch(() => {
        if (!cancelled) setSession({ active: false });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [errorMessage, successData, sessionId]);

  const handleSelectSpool = React.useCallback(async (spoolId: number) => {
    setSelecting(true);
    try {
      const result = await api.selectNfcSpool(spoolId, sessionId);
      if (result.completed && result.success) {
        setPickedSuccess(selectSuccessToScanData(result.success));
        setSession({ active: false });
        return;
      }
      if (result.session) {
        setSession(result.session);
      }
    } catch {
      setSession((current) => current);
    } finally {
      setSelecting(false);
    }
  }, [sessionId]);

  const accent =
    successData || errorMessage
      ? undefined
      : accentFromSession(session ?? { active: false });

  if (errorMessage) {
    return (
      <ScanLayout>
        <ScanErrorState message={errorMessage} />
      </ScanLayout>
    );
  }

  if (successData) {
    return (
      <ScanLayout accentColor="oklch(0.72 0.19 155)">
        <ScanSuccess data={successData} />
      </ScanLayout>
    );
  }

  if (loading) {
    return (
      <ScanLayout accentColor={accent}>
        <div className="flex flex-1 items-center justify-center">
          <div className="size-8 animate-spin rounded-full border-2 border-white/20 border-t-white/70" />
        </div>
      </ScanLayout>
    );
  }

  if (!session?.active) {
    return (
      <ScanLayout>
        <ScanEmptyState />
      </ScanLayout>
    );
  }

  if (session.has_pending_filament && session.pending_filament) {
    return (
      <ScanLayout accentColor={accent}>
        <ScanSpoolPicker
          filament={session.pending_filament}
          location={session.location}
          selecting={selecting}
          onSelect={handleSelectSpool}
        />
      </ScanLayout>
    );
  }

  return (
    <ScanLayout accentColor={accent}>
      <ScanPairingView session={session} />
    </ScanLayout>
  );
}
