"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { api } from "./api";
import type { StatusMessage } from "./types";

interface StatusSocketState {
  status: StatusMessage | null;
  connected: boolean;
  refresh: () => void;
}

const DEFAULT_POLL_INTERVAL_MS = 30_000;

// Live status via /ws/status (proxied by Next), with HTTP polling at poll_interval
// to keep the UI in sync even when broadcasts are missed.
export function useStatusSocket(
  pollIntervalMs: number = DEFAULT_POLL_INTERVAL_MS
): StatusSocketState {
  const [status, setStatus] = useState<StatusMessage | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const retryRef = useRef(0);

  const fetchSnapshot = useCallback(async () => {
    try {
      const [statusData, spools, errors] = await Promise.all([
        api.getStatus(),
        api.getSpools().catch(() => []),
        api.getPrintErrors().catch(() => ({ errors: [] })),
      ]);
      setStatus({
        type: "status_update",
        timestamp: statusData.timestamp,
        printers: statusData.printers ?? {},
        toolhead_mappings: statusData.toolhead_mappings ?? {},
        spools: spools ?? [],
        print_errors: errors.errors ?? [],
      });
    } catch {
      // backend unreachable — keep last known state
    }
  }, []);

  useEffect(() => {
    let closed = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

    // Initial snapshot so the UI renders before the first broadcast.
    const initialTimer = setTimeout(fetchSnapshot, 0);

    const pollTimer = setInterval(fetchSnapshot, pollIntervalMs);

    const connect = () => {
      if (closed) return;
      const proto = window.location.protocol === "https:" ? "wss" : "ws";
      const ws = new WebSocket(`${proto}://${window.location.host}/ws/status`);
      wsRef.current = ws;

      ws.onopen = () => {
        retryRef.current = 0;
        setConnected(true);
      };

      ws.onmessage = (event) => {
        for (const chunk of String(event.data).split("\n")) {
          if (!chunk.trim()) continue;
          try {
            const message = JSON.parse(chunk) as StatusMessage;
            if (message.type === "status_update") {
              setStatus(message);
            }
          } catch {
            // ignore malformed frames
          }
        }
      };

      ws.onclose = () => {
        setConnected(false);
        if (closed) return;
        retryRef.current += 1;
        const delay = Math.min(15000, 1000 * 2 ** Math.min(retryRef.current, 4));
        reconnectTimer = setTimeout(connect, delay);
      };

      ws.onerror = () => {
        ws.close();
      };
    };

    connect();

    return () => {
      closed = true;
      clearTimeout(initialTimer);
      clearInterval(pollTimer);
      if (reconnectTimer) clearTimeout(reconnectTimer);
      wsRef.current?.close();
    };
  }, [fetchSnapshot, pollIntervalMs]);

  return { status, connected, refresh: fetchSnapshot };
}
