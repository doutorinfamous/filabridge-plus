"use client";

import * as React from "react";

export type NfcWriteStatus = "idle" | "writing" | "success" | "error";

export interface NfcWriterState {
  /** null while unresolved (SSR / before hydration), then true/false. */
  supported: boolean | null;
  status: NfcWriteStatus;
  /** User-friendly error message when status === "error". */
  errorMessage: string | null;
  write: (url: string) => Promise<void>;
  cancel: () => void;
  reset: () => void;
}

const WRITE_TIMEOUT_MS = 20_000;

function friendlyErrorMessage(error: unknown): string {
  if (error instanceof DOMException) {
    switch (error.name) {
      case "NotAllowedError":
        return "NFC permission was denied. Allow NFC access in your browser and try again.";
      case "NotSupportedError":
        return "This device has no compatible NFC hardware.";
      case "NotReadableError":
        return "Could not access NFC. Make sure NFC is turned on in your device settings.";
      case "AbortError":
        return "Writing was cancelled or timed out. Hold the tag near your phone and try again.";
      case "NetworkError":
        return "The tag moved away or could not be written. Hold it steady near your phone and try again.";
      case "InvalidStateError":
        return "The tag could not be written. It may be read-only or too small for this URL.";
      case "SecurityError":
        return "NFC writing requires a secure connection (HTTPS).";
    }
  }
  return "Could not write the NFC tag. Please try again.";
}

/**
 * Reusable client-only hook for writing a URL to an NFC tag via the
 * Web NFC API (NDEFReader). Only supported on Chrome/Edge for Android.
 */
export function useNfcWriter(): NfcWriterState {
  const [supported, setSupported] = React.useState<boolean | null>(null);
  const [status, setStatus] = React.useState<NfcWriteStatus>("idle");
  const [errorMessage, setErrorMessage] = React.useState<string | null>(null);
  const abortRef = React.useRef<AbortController | null>(null);
  const timeoutRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  React.useEffect(() => {
    setSupported(typeof window !== "undefined" && "NDEFReader" in window);
  }, []);

  const clearTimer = React.useCallback(() => {
    if (timeoutRef.current !== null) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
  }, []);

  const cancel = React.useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    clearTimer();
    setStatus("idle");
    setErrorMessage(null);
  }, [clearTimer]);

  const reset = React.useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    clearTimer();
    setStatus("idle");
    setErrorMessage(null);
  }, [clearTimer]);

  // Abort any pending write on unmount.
  React.useEffect(() => {
    return () => {
      abortRef.current?.abort();
      if (timeoutRef.current !== null) clearTimeout(timeoutRef.current);
    };
  }, []);

  const write = React.useCallback(
    async (url: string) => {
      if (typeof window === "undefined" || !window.NDEFReader) {
        setStatus("error");
        setErrorMessage("Web NFC is not supported by this browser.");
        return;
      }

      // Abort a previous attempt, if any.
      abortRef.current?.abort();
      clearTimer();

      const controller = new AbortController();
      abortRef.current = controller;
      timeoutRef.current = setTimeout(() => controller.abort(), WRITE_TIMEOUT_MS);

      setStatus("writing");
      setErrorMessage(null);

      try {
        const ndef = new window.NDEFReader();
        await ndef.write(
          { records: [{ recordType: "url", data: url }] },
          { signal: controller.signal }
        );
        setStatus("success");
      } catch (error) {
        console.error("[Web NFC] Failed to write NFC tag:", error);
        // If this attempt was superseded or cancelled by the user, stay quiet.
        if (abortRef.current !== controller) return;
        setStatus("error");
        setErrorMessage(friendlyErrorMessage(error));
      } finally {
        if (abortRef.current === controller) {
          abortRef.current = null;
          clearTimer();
        }
      }
    },
    [clearTimer]
  );

  return { supported, status, errorMessage, write, cancel, reset };
}
