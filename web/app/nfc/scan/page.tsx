import { Suspense } from "react";

import NfcScanContent from "./nfc-scan-content";

export default function NfcScanPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-dvh items-center justify-center bg-[oklch(0.11_0.02_270)]">
          <div className="size-8 animate-spin rounded-full border-2 border-white/20 border-t-white/70" />
        </div>
      }
    >
      <NfcScanContent />
    </Suspense>
  );
}
