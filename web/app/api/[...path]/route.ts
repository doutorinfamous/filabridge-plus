import { NextRequest } from "next/server";

// Reverse proxy for the Go backend: keeps every /api/* path identical to the
// original FilaBridge API while exposing a single external port (5000).
const BACKEND_URL = process.env.BACKEND_URL ?? "http://127.0.0.1:5001";

export const dynamic = "force-dynamic";

async function proxy(
  req: NextRequest,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const { path } = await params;
  const url = new URL(req.url);
  const target = `${BACKEND_URL}/api/${path.map(encodeURIComponent).join("/")}${url.search}`;

  const headers = new Headers(req.headers);
  // Hop-by-hop headers must not be forwarded (undici also rejects some of them).
  for (const h of [
    "host",
    "connection",
    "content-length",
    "expect",
    "keep-alive",
    "proxy-connection",
    "transfer-encoding",
    "te",
    "trailer",
    "upgrade",
  ]) {
    headers.delete(h);
  }
  // Let the Node fetch negotiate encoding; avoids double-compression issues.
  headers.delete("accept-encoding");
  // Preserve the host the browser used so the backend can build NFC/QR URLs.
  headers.set("x-forwarded-host", url.host);
  headers.set("x-forwarded-proto", url.protocol.replace(":", ""));

  const clientIp =
    req.headers.get("x-forwarded-for")?.split(",")[0]?.trim() ||
    req.headers.get("x-real-ip");
  if (clientIp) {
    headers.set("x-forwarded-for", clientIp);
    headers.set("x-real-ip", clientIp);
  }

  const hasBody = req.method !== "GET" && req.method !== "HEAD";

  const res = await fetch(target, {
    method: req.method,
    headers,
    body: hasBody ? req.body : undefined,
    // @ts-expect-error -- duplex is required by Node fetch for streaming bodies
    duplex: hasBody ? "half" : undefined,
    redirect: "manual",
    cache: "no-store",
  });

  const resHeaders = new Headers(res.headers);
  resHeaders.delete("content-encoding");
  resHeaders.delete("content-length");
  resHeaders.delete("transfer-encoding");
  resHeaders.delete("connection");

  return new Response(res.body, {
    status: res.status,
    statusText: res.statusText,
    headers: resHeaders,
  });
}

export {
  proxy as GET,
  proxy as POST,
  proxy as PUT,
  proxy as DELETE,
  proxy as PATCH,
  proxy as HEAD,
  proxy as OPTIONS,
};
