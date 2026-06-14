import type { NextConfig } from "next";

// Go backend (FilaBridge API). In production (Docker) both processes run in the
// same container: Next.js listens on :5000 and the Go API on 127.0.0.1:5001.
const backendUrl = process.env.BACKEND_URL ?? "http://127.0.0.1:5001";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      // Status WebSocket: the self-hosted Next server proxies upgrade requests
      // for external (http://) rewrites — see router-server upgradeHandler.
      // Regular HTTP API calls go through app/api/[...path]/route.ts instead.
      { source: "/ws/:path*", destination: `${backendUrl}/ws/:path*` },
    ];
  },
};

export default nextConfig;
