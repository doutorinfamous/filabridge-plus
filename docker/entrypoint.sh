#!/bin/sh
# Starts the Go API (internal :5001) and the Next.js server (:5000).
# If either process dies, the container exits so the orchestrator restarts it.
set -e

DATA_DIR="${FILABRIDGE_DB_PATH:-/app/data}"
SEED_DIR="/app/seed"
DB_NAME="filabridge.db"

mkdir -p "$DATA_DIR"

# Seed the Docker volume once from the local dev database (read-only mount).
if [ ! -f "$DATA_DIR/$DB_NAME" ] && [ -f "$SEED_DIR/$DB_NAME" ]; then
  cp "$SEED_DIR/$DB_NAME" "$DATA_DIR/$DB_NAME"
  echo "Seeded $DATA_DIR/$DB_NAME from $SEED_DIR/$DB_NAME"
fi

shutdown() {
  kill -TERM "$GO_PID" "$NODE_PID" 2>/dev/null || true
}
trap shutdown TERM INT

./filabridge --host 127.0.0.1 --port 5001 &
GO_PID=$!

PORT=5000 HOSTNAME=0.0.0.0 node web/server.js &
NODE_PID=$!

while kill -0 "$GO_PID" 2>/dev/null && kill -0 "$NODE_PID" 2>/dev/null; do
  sleep 2
done

shutdown
wait 2>/dev/null || true
