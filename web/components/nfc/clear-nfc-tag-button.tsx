"use client";

import * as React from "react";
import { Check, Eraser, Loader2, RotateCcw, X } from "lucide-react";
import { toast } from "sonner";

import { useNfcWriter } from "@/hooks/use-nfc-writer";
import { Button } from "@/components/ui/button";

/**
 * Clears an NFC tag by writing an empty NDEF record via the Web NFC API.
 * Only supported on Chrome/Edge for Android.
 */
export function ClearNfcTagButton() {
  const { supported, status, errorMessage, erase, cancel } = useNfcWriter();

  React.useEffect(() => {
    if (status === "success") {
      toast.success("NFC tag cleared — you can write a new URL");
    } else if (status === "error" && errorMessage) {
      toast.error(errorMessage);
    }
  }, [status, errorMessage]);

  if (supported === null) return null;

  if (!supported) {
    return (
      <p className="max-w-md text-center text-xs text-muted-foreground">
        Direct NFC clearing isn&apos;t supported by this browser (requires
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
      <Button variant="outline" onClick={() => erase()}>
        {status === "success" ? (
          <>
            <Check className="size-4 text-success" /> Tag cleared — clear
            another
          </>
        ) : status === "error" ? (
          <>
            <RotateCcw className="size-4" /> Try clearing again
          </>
        ) : (
          <>
            <Eraser className="size-4" /> Clear tag content
          </>
        )}
      </Button>
      <p className="text-center text-xs text-muted-foreground">
        Removes the current URL from a writable tag so you can program it
        again. Read-only or locked tags cannot be cleared.
      </p>
      {status === "error" && errorMessage ? (
        <p className="text-center text-xs text-destructive">{errorMessage}</p>
      ) : null}
    </div>
  );
}
