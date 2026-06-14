# FilaBridge+ Web (Next.js)

FilaBridge+ front-end built with **Next.js 16**, **Tailwind CSS v4**, and
**shadcn/ui**. Serves the UI on port **5000** and proxies the full API to the
Go backend (internal port 5001), keeping original paths:

- `/api/*` → proxy via route handler ([app/api/[...path]/route.ts](app/api/%5B...path%5D/route.ts))
- `/ws/*` → WebSocket proxy via rewrite ([next.config.ts](next.config.ts))

## Development

```bash
# 1. Go backend (in another terminal)
cd ../backend && go run . --port 5001

# 2. Front-end
npm install
npm run dev -- -p 5000
```

Open http://localhost:5000. Backend URL can be changed with the
`BACKEND_URL` variable (default: `http://127.0.0.1:5001`).

## Structure

- `app/` — pages: Dashboard (`/`), NFC (`/nfc`), Settings (`/settings`)
- `components/` — printer cards (Moonraker/Bambu), spool combobox, etc.
- `components/ui/` — shadcn/ui components
- `lib/` — typed API client, types, and status WebSocket hook

## Production build

```bash
npm run build   # standalone output (used by root Dockerfile)
```
