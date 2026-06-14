// Smoke test: connects to the status WebSocket through the Next.js proxy and
// waits for the first status_update broadcast. Usage: node ws-smoke.mjs [url]
const url = process.argv[2] ?? "ws://localhost:5000/ws/status";

const ws = new WebSocket(url);
const timeout = setTimeout(() => {
  console.error("TIMEOUT: no status_update received in 45s");
  process.exit(2);
}, 45_000);

ws.onopen = () => console.log("OPEN: websocket connected via proxy");
ws.onerror = (err) => {
  console.error("ERROR:", err.message ?? err);
  process.exit(1);
};
ws.onmessage = (event) => {
  const message = JSON.parse(String(event.data).split("\n")[0]);
  console.log(
    `MESSAGE: type=${message.type} printers=${Object.keys(message.printers ?? {}).length}`
  );
  clearTimeout(timeout);
  ws.close();
  process.exit(0);
};
