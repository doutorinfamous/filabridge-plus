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
  erase: () => Promise<void>;
  cancel: () => void;
  reset: () => void;
}

const WRITE_TIMEOUT_MS = 20_000;

function friendlyErrorMessage(
  error: unknown,
  mode: "write" | "erase"
): string {
  if (error instanceof DOMException) {
    switch (error.name) {
      case "NotAllowedError":
        return "NFC permission was denied. Allow NFC access in your browser and try again.";
      case "NotSupportedError":
        return "This device has no compatible NFC hardware.";
      case "NotReadableError":
        return "Could not access NFC. Make sure NFC is turned on in your device settings.";
      case "AbortError":
        return mode === "erase"
          ? "Clearing was cancelled or timed out. Hold the tag near your phone and try again."
          : "Writing was cancelled or timed out. Hold the tag near your phone and try again.";
      case "NetworkError":
        return mode === "erase"
          ? "The tag moved away or could not be cleared. Hold it steady near your phone and try again."
          : "The tag moved away or could not be written. Hold it steady near your phone and try again.";
      case "InvalidStateError":
        return mode === "erase"
          ? "The tag could not be cleared. It may be read-only or locked."
          : "The tag could not be written. It may be read-only or too small for this URL.";
      case "SecurityError":
        return "NFC operations require a secure connection (HTTPS).";
    }
  }
  return mode === "erase"
    ? "Could not clear the NFC tag. Please try again."
    : "Could not write the NFC tag. Please try again.";
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

  // Abort any pending operation on unmount.
  React.useEffect(() => {
    return () => {
      abortRef.current?.abort();
      if (timeoutRef.current !== null) clearTimeout(timeoutRef.current);
    };
  }, []);

  const runOperation = React.useCallback(
    async (message: NDEFMessageInit, mode: "write" | "erase") => {
      if (typeof window === "undefined" || !window.NDEFReader) {
        setStatus("error");
        setErrorMessage("Web NFC is not supported by this browser.");
        return;
      }

      abortRef.current?.abort();
      clearTimer();

      const controller = new AbortController();
      abortRef.current = controller;
      timeoutRef.current = setTimeout(
        () => controller.abort(),
        WRITE_TIMEOUT_MS
      );

      setStatus("writing");
      setErrorMessage(null);

      try {
        const ndef = new window.NDEFReader();
        await ndef.write(message, { signal: controller.signal });
        setStatus("success");
      } catch (error) {
        console.error(`[Web NFC] Failed to ${mode} NFC tag:`, error);
        if (abortRef.current !== controller) return;
        setStatus("error");
        setErrorMessage(friendlyErrorMessage(error, mode));
      } finally {
        if (abortRef.current === controller) {
          abortRef.current = null;
          clearTimer();
        }
      }
    },
    [clearTimer]
  );

  const write = React.useCallback(
    async (url: string) => {
      await runOperation({ records: [{ recordType: "url", data: url }] }, "write");
    },
    [runOperation]
  );

  const erase = React.useCallback(async () => {
    await runOperation({ records: [{ recordType: "empty" }] }, "erase");
  }, [runOperation]);

  return { supported, status, errorMessage, write, erase, cancel, reset };
}
