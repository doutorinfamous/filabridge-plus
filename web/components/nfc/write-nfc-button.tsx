"use client";

import * as React from "react";
import { Check, Loader2, Nfc, RotateCcw, X } from "lucide-react";
import { toast } from "sonner";

import { useNfcWriter } from "@/hooks/use-nfc-writer";
import { Button } from "@/components/ui/button";

interface WriteNfcButtonProps {
  url: string;
}

/**
 * Optional "write directly to an NFC tag" action using the Web NFC API.
 * Only rendered as an active button when the browser supports NDEFReader
 * (Chrome/Edge on Android); otherwise shows a short unsupported note.
 */
export function WriteNfcButton({ url }: WriteNfcButtonProps) {
  const { supported, status, errorMessage, write, cancel } = useNfcWriter();

  React.useEffect(() => {
    if (status === "success") {
      toast.success("NFC tag written successfully");
    } else if (status === "error" && errorMessage) {
      toast.error(errorMessage);
    }
  }, [status, errorMessage]);

  // Unresolved during SSR / before hydration: render nothing.
  if (supported === null) return null;

  if (!supported) {
    return (
      <p className="max-w-md text-center text-xs text-muted-foreground">
        Direct NFC writing isn&apos;t supported by this browser (requires
        Chrome on Android). Use the manual steps below instead.
      </p>
    );
  }

  if (status === "writing") {
    return (
      <div className="flex w-full max-w-md flex-col items-center gap-2">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" />
          Hold the NFC tag near your phone…
        </div>
        <Button variant="outline" size="sm" onClick={cancel}>
          <X className="size-4" /> Cancel
        </Button>
      </div>
    );
  }

  return (
    <div className="flex w-full max-w-md flex-col items-center gap-2">
      <Button variant="secondary" onClick={() => write(url)}>
        {status === "success" ? (
          <>
            <Check className="size-4 text-success" /> Tag written — write
            another
          </>
        ) : status === "error" ? (
          <>
            <RotateCcw className="size-4" /> Try writing again
          </>
        ) : (
          <>
            <Nfc className="size-4" /> Write to NFC tag
          </>
        )}
      </Button>
      {status === "error" && errorMessage ? (
        <p className="text-center text-xs text-destructive">{errorMessage}</p>
      ) : null}
    </div>
  );
}
