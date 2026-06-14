"use client";

import { useEffect, useState } from "react";

import { api } from "./api";

const DEFAULT_POLL_INTERVAL_SEC = 30;
const MIN_POLL_INTERVAL_SEC = 10;
const MAX_POLL_INTERVAL_SEC = 300;

function parsePollInterval(value: string | undefined): number {
  const parsed = parseInt(value ?? String(DEFAULT_POLL_INTERVAL_SEC), 10);
  if (Number.isNaN(parsed)) return DEFAULT_POLL_INTERVAL_SEC;
  return Math.min(MAX_POLL_INTERVAL_SEC, Math.max(MIN_POLL_INTERVAL_SEC, parsed));
}

export function usePollInterval() {
  const [intervalSec, setIntervalSec] = useState(DEFAULT_POLL_INTERVAL_SEC);

  useEffect(() => {
    let cancelled = false;

    api
      .getConfig()
      .then((cfg) => {
        if (!cancelled) {
          setIntervalSec(parsePollInterval(cfg.poll_interval));
        }
      })
      .catch(() => {
        // keep default
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return {
    intervalSec,
    intervalMs: intervalSec * 1000,
  };
}
